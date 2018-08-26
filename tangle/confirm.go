package tangle

import (
	"bytes"
	"time"

	"encoding/gob"
	"sync"

	"../convert"
	"../db"
	"../logs"
	"../snapshot"
	"../transaction"
	"github.com/pkg/errors"
)

const CONFIRM_CHECK_INTERVAL = time.Duration(100) * time.Millisecond
const UNKNOWN_CHECK_INTERVAL = time.Duration(60) * time.Second

type PendingConfirmation struct {
	key       []byte
	timestamp int
}
type ConfirmQueue chan PendingConfirmation

var confirmLocker = &sync.RWMutex{}
var confirmsInProgress map[string]bool
var confirmQueue ConfirmQueue

func confirmOnLoad() {
	logs.Log.Info("Starting confirmation thread")
	confirmsInProgress = make(map[string]bool)
	confirmQueue = make(ConfirmQueue, maxQueueSize)
	loadPendingConfirmations()
	logs.Log.Infof("Loaded %v pending confirmations", len(confirmQueue))
	go startUnknownVerificationThread()
	for i := 0; i < nbWorkers; i++ {
		go startConfirmThread()
	}
}

func loadPendingConfirmations() {
	db.Singleton.View(func(tx db.Transaction) error {
		tx.ForPrefix([]byte{db.KEY_EVENT_CONFIRMATION_PENDING}, true, func(key, value []byte) (bool, error) {
			var timestamp int
			if err := gob.NewDecoder(bytes.NewBuffer(value)).Decode(&timestamp); err != nil {
				logs.Log.Error("Couldn't load pending confirmation key value", key, err)
				return true, nil
			}
			confirmQueue <- PendingConfirmation{
				db.AsKey(key, db.KEY_EVENT_CONFIRMATION_PENDING),
				timestamp,
			}
			return true, nil
		})
		return nil
	})
}

func startConfirmThread() {
	for {
		if (lowEndDevice && len(srv.Incoming) > maxIncoming) || len(confirmQueue) < 1 {
			time.Sleep(CONFIRM_CHECK_INTERVAL)
			continue
		}
		pendingConfirmation := <-confirmQueue
		db.Singleton.Lock()
		db.Singleton.Unlock()
		confirmed := false
		err := db.Singleton.Update(func(tx db.Transaction) error {
			if !addConfirmInProgress(pendingConfirmation.key) {
				return nil
			}
			err, c := confirm(pendingConfirmation.key, tx)
			confirmed = c
			removeConfirmInProgress(pendingConfirmation.key)
			return err
		})
		if err != nil || db.Singleton.HasKey(pendingConfirmation.key) {
			confirmQueue <- pendingConfirmation
		} else if confirmed {
			totalConfirmations++
		}
	}
}

func startUnknownVerificationThread() {
	flushTicker := time.NewTicker(UNKNOWN_CHECK_INTERVAL)
	for range flushTicker.C {
		db.Singleton.View(func(tx db.Transaction) error {
			var toRemove [][]byte
			tx.ForPrefix([]byte{db.KEY_PENDING_CONFIRMED}, false, func(key, _ []byte) (bool, error) {
				if tx.HasKey(db.AsKey(key, db.KEY_HASH)) {
					k := make([]byte, len(key))
					copy(k, key)
					toRemove = append(toRemove, k)
				}
				return true, nil
			})

			for _, key := range toRemove {
				logs.Log.Debug("Removing orphaned pending confirmed key", key)
				err := db.Singleton.Update(func(tx db.Transaction) error {
					if err := tx.Remove(key); err != nil {
						return err
					}
					return confirmChild(db.AsKey(key, db.KEY_HASH), tx)
				})
				if err != nil {
					return err
				}
			}
			return nil
		})
	}
}

func confirm(key []byte, tx db.Transaction) (error, bool) {
	tx.Remove(db.AsKey(key, db.KEY_EVENT_CONFIRMATION_PENDING))

	if tx.HasKey(db.AsKey(key, db.KEY_CONFIRMED)) {
		return nil, false
	}

	data, err := tx.GetBytes(db.AsKey(key, db.KEY_BYTES))
	if err != nil {
		// Imminent database inconsistency: Warn!
		// logs.Log.Error("TX missing for confirmation. Probably snapshotted. DB inconsistency imminent!", key)
		return errors.New("TX  missing for confirmation!"), false
	}
	trits := convert.BytesToTrits(data)[:8019]
	var t = transaction.TritsToFastTX(&trits, data)

	if tx.HasKey(db.AsKey(key, db.KEY_EVENT_TRIM_PENDING)) && !isMaybeMilestonePart(t) {
		logs.Log.Errorf("TX behind snapshot horizon, skipping (%v vs %v). Possible DB inconsistency! TX: %v",
			t.Timestamp,
			snapshot.GetSnapshotTimestamp(tx),
			convert.BytesToTrytes(t.Hash))
		return nil, false
	}

	err = tx.Put(db.AsKey(key, db.KEY_CONFIRMED), t.Timestamp, nil)
	if err != nil {
		logs.Log.Errorf("Could not save confirmation status!", err)
		return errors.New("Could not save confirmation status!"), false
	}

	if t.Value != 0 {
		_, err := tx.IncrementBy(db.GetAddressKey(t.Address, db.KEY_BALANCE), t.Value, false)
		if err != nil {
			logs.Log.Errorf("Could not update account balance: %v", err)
			return errors.New("Could not update account balance!"), false
		}
		if t.Value < 0 {
			err := tx.Put(db.GetAddressKey(t.Address, db.KEY_SPENT), true, nil)
			if err != nil {
				logs.Log.Errorf("Could not update account spent status: %v", err)
				return errors.New("Could not update account spent status!"), false
			}
		}
	}

	err = confirmChild(db.GetByteKey(t.TrunkTransaction, db.KEY_HASH), tx)
	if err != nil {
		return err, false
	}
	err = confirmChild(db.GetByteKey(t.BranchTransaction, db.KEY_HASH), tx)
	if err != nil {
		return err, false
	}
	return nil, true
}

func confirmChild(key []byte, tx db.Transaction) error {
	if bytes.Equal(key, tipHashKey) {
		return nil
	}
	if tx.HasKey(db.AsKey(key, db.KEY_CONFIRMED)) {
		return nil
	}
	timestamp, err := tx.GetInt(db.AsKey(key, db.KEY_TIMESTAMP))
	k := db.AsKey(key, db.KEY_EVENT_CONFIRMATION_PENDING)
	if err == nil && !hasConfirmInProgress(k) {
		err = addPendingConfirmation(k, timestamp, tx)
		if err != nil {
			logs.Log.Errorf("Could not save child confirm status: %v", err)
			return errors.New("Could not save child confirm status!")
		}
	} else if !tx.HasKey(db.AsKey(key, db.KEY_EDGE)) && tx.HasKey(db.AsKey(key, db.KEY_PENDING_HASH)) {
		err = tx.Put(db.AsKey(key, db.KEY_PENDING_CONFIRMED), int(time.Now().Unix()), nil)
		if err != nil {
			logs.Log.Errorf("Could not save child pending confirm status: %v", err)
			return errors.New("Could not save child pending confirm status!")
		}
	}
	return nil
}

func addPendingConfirmation(key []byte, timestamp int, tx db.Transaction) error {
	err := tx.Put(db.AsKey(key, db.KEY_EVENT_CONFIRMATION_PENDING), timestamp, nil)
	if err == nil {
		confirmQueue <- PendingConfirmation{key, timestamp}
	}
	return err
}

func hasConfirmInProgress(key []byte) bool {
	confirmLocker.Lock()
	defer confirmLocker.Unlock()

	_, ok := confirmsInProgress[string(key)]
	return ok
}

func addConfirmInProgress(key []byte) bool {
	if hasConfirmInProgress(key) {
		return false
	}

	confirmLocker.Lock()
	defer confirmLocker.Unlock()

	confirmsInProgress[string(key)] = true
	return true
}

func removeConfirmInProgress(key []byte) {
	confirmLocker.Lock()
	defer confirmLocker.Unlock()
	k := string(key)
	_, ok := confirmsInProgress[k]
	if ok {
		delete(confirmsInProgress, k)
	}
}

func reapplyConfirmed() {
	logs.Log.Debug("Reapplying confirmed TXs to balances")
	db.Singleton.View(func(tx db.Transaction) error {
		x := 0
		return tx.ForPrefix([]byte{db.KEY_CONFIRMED}, false, func(key, _ []byte) (bool, error) {
			txBytes, _ := tx.GetBytes(db.AsKey(key, db.KEY_BYTES))
			trits := convert.BytesToTrits(txBytes)[:8019]
			t := transaction.TritsToFastTX(&trits, txBytes)
			if t.Value != 0 {
				err := db.Singleton.Update(func(tx db.Transaction) error {
					_, err := tx.IncrementBy(db.GetAddressKey(t.Address, db.KEY_BALANCE), t.Value, false)
					if err != nil {
						logs.Log.Errorf("Could not update account balance: %v", err)
						return errors.New("Could not update account balance!")
					}
					if t.Value < 0 {
						err := tx.Put(db.GetAddressKey(t.Address, db.KEY_SPENT), true, nil)
						if err != nil {
							logs.Log.Errorf("Could not update account spent status: %v", err)
							return errors.New("Could not update account spent status!")
						}
					}
					return nil
				})
				if err != nil {
					logs.Log.Errorf("Could not apply tx Value: %v", err)
					return false, errors.New("Could not apply Tx value!")
				}
			}
			x = x + 1
			if x%10000 == 0 {
				logs.Log.Debug("Progress", x)
			}
			return true, nil
		})
	})
}
