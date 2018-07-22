package tangle

import (
	"bytes"
	"time"

	"../convert"
	"../db"
	"../logs"
	"../snapshot"
	"../transaction"
	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
	"sync"
	"encoding/gob"
)

const CONFIRM_CHECK_INTERVAL = time.Duration(100) * time.Millisecond
const UNKNOWN_CHECK_INTERVAL = time.Duration(60) * time.Second

type PendingConfirmation struct {
	key []byte
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
	_ = db.DB.View(func(txn *badger.Txn) (e error) {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte{db.KEY_EVENT_CONFIRMATION_PENDING}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := item.Key()
			value, err := item.Value()
			if err != nil {
				logs.Log.Error("Couldn't load pending confirmation key", key)
				continue
			}
			var timestamp int
			buf := bytes.NewBuffer(value)
			dec := gob.NewDecoder(buf)
			err = dec.Decode(&timestamp)
			if err != nil {
				logs.Log.Error("Couldn't load pending confirmation key value", key, err)
				continue
			}
			confirmQueue <- PendingConfirmation{
				db.AsKey(key, db.KEY_EVENT_CONFIRMATION_PENDING),
				timestamp,
			}
		}
		return nil
	})
}

func startConfirmThread() {
	for {
		if (lowEndDevice && len(srv.Incoming) > maxIncoming) || len(confirmQueue) < 1 {
			time.Sleep(CONFIRM_CHECK_INTERVAL)
			continue
		}
		pendingConfirmation := <- confirmQueue
		db.Locker.Lock()
		db.Locker.Unlock()
		confirmed := false
		err := db.DB.Update(func(txn *badger.Txn) error {
			if !addConfirmInProgress(pendingConfirmation.key) {
				return nil
			}
			err, c := confirm(pendingConfirmation.key, txn)
			confirmed = c
			removeConfirmInProgress(pendingConfirmation.key)
			return err
		})
		if err != nil || db.Has(pendingConfirmation.key, nil) {
			confirmQueue <- pendingConfirmation
		} else if confirmed {
			totalConfirmations++
		}
	}
}

func startUnknownVerificationThread() {
	flushTicker := time.NewTicker(UNKNOWN_CHECK_INTERVAL)
	for range flushTicker.C {
		_ = db.DB.View(func(txn *badger.Txn) error {
			opts := badger.DefaultIteratorOptions
			it := txn.NewIterator(opts)
			defer it.Close()
			prefix := []byte{db.KEY_PENDING_CONFIRMED}
			var toRemove [][]byte
			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				key := it.Item().Key()
				if db.Has(db.AsKey(key, db.KEY_HASH), txn) {
					k := make([]byte, len(key))
					copy(k, key)
					toRemove = append(toRemove, k)
				}
			}
			for _, key := range toRemove {
				logs.Log.Debug("Removing orphaned pending confirmed key", key)
				err := db.DB.Update(func(txn *badger.Txn) error {
					err := db.Remove(key, txn)
					if err != nil { return err }
					return confirmChild(db.AsKey(key, db.KEY_HASH), txn)
				})
				if err != nil { return err }
			}
			return nil
		})
	}
}

func confirm(key []byte, txn *badger.Txn) (error, bool) {
	db.Remove(db.AsKey(key, db.KEY_EVENT_CONFIRMATION_PENDING), txn)

	if db.Has(db.AsKey(key, db.KEY_CONFIRMED), txn) {
		return nil, false
	}

	data, err := db.GetBytes(db.AsKey(key, db.KEY_BYTES), txn)
	if err != nil {
		// Imminent database inconsistency: Warn!
		// logs.Log.Error("TX missing for confirmation. Probably snapshotted. DB inconsistency imminent!", key)
		return errors.New("TX  missing for confirmation!"), false
	}
	trits := convert.BytesToTrits(data)[:8019]
	var tx = transaction.TritsToFastTX(&trits, data)

	if db.Has(db.AsKey(key, db.KEY_EVENT_TRIM_PENDING), txn) && !isMaybeMilestonePart(tx) {
		logs.Log.Errorf("TX behind snapshot horizon, skipping (%v vs %v). Possible DB inconsistency! TX: %v",
			tx.Timestamp,
			snapshot.GetSnapshotTimestamp(txn),
			convert.BytesToTrytes(tx.Hash)[:81])
		return nil, false
	}

	err = db.Put(db.AsKey(key, db.KEY_CONFIRMED), tx.Timestamp, nil, txn)

	if err != nil {
		logs.Log.Errorf("Could not save confirmation status!", err)
		return errors.New("Could not save confirmation status!"), false
	}

	if tx.Value != 0 {
		_, err := db.IncrBy(db.GetAddressKey(tx.Address, db.KEY_BALANCE), tx.Value, false, txn)
		if err != nil {
			logs.Log.Errorf("Could not update account balance: %v", err)
			return errors.New("Could not update account balance!"), false
		}
		if tx.Value < 0 {
			err := db.Put(db.GetAddressKey(tx.Address, db.KEY_SPENT), true, nil, txn)
			if err != nil {
				logs.Log.Errorf("Could not update account spent status: %v", err)
				return errors.New("Could not update account spent status!"), false
			}
		}
	}

	err = confirmChild(db.GetByteKey(tx.TrunkTransaction, db.KEY_HASH), txn)
	if err != nil {
		return err, false
	}
	err = confirmChild(db.GetByteKey(tx.BranchTransaction, db.KEY_HASH), txn)
	if err != nil {
		return err, false
	}
	return nil, true
}

func confirmChild(key []byte, txn *badger.Txn) error {
	if bytes.Equal(key, tipHashKey) {
		return nil
	}
	if db.Has(db.AsKey(key, db.KEY_CONFIRMED), txn) {
		return nil
	}
	timestamp, err := db.GetInt(db.AsKey(key, db.KEY_TIMESTAMP), txn)
	k := db.AsKey(key, db.KEY_EVENT_CONFIRMATION_PENDING)
	if err == nil && !hasConfirmInProgress(k) {
		err = addPendingConfirmation(k, timestamp, txn)
		if err != nil {
			logs.Log.Errorf("Could not save child confirm status: %v", err)
			return errors.New("Could not save child confirm status!")
		}
	} else if !db.Has(db.AsKey(key, db.KEY_EDGE), txn) && db.Has(db.AsKey(key, db.KEY_PENDING_HASH), txn) {
		err = db.Put(db.AsKey(key, db.KEY_PENDING_CONFIRMED), int(time.Now().Unix()), nil, txn)
		if err != nil {
			logs.Log.Errorf("Could not save child pending confirm status: %v", err)
			return errors.New("Could not save child pending confirm status!")
		}
	}
	return nil
}

func addPendingConfirmation(key []byte, timestamp int, txn *badger.Txn) error {
	err := db.Put(db.AsKey(key, db.KEY_EVENT_CONFIRMATION_PENDING), timestamp, nil, txn)
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

func addConfirmInProgress (key []byte) bool {
	if hasConfirmInProgress(key) {
		return false
	}

	confirmLocker.Lock()
	defer confirmLocker.Unlock()

	confirmsInProgress[string(key)] = true
	return true
}

func removeConfirmInProgress (key []byte) {
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
	db.DB.View(func(txn *badger.Txn) (e error) {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte{db.KEY_CONFIRMED}
		x := 0
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := item.Key()
			txBytes, _ := db.GetBytes(db.AsKey(key, db.KEY_BYTES), txn)
			trits := convert.BytesToTrits(txBytes)[:8019]
			tx := transaction.TritsToFastTX(&trits, txBytes)
			if tx.Value != 0 {
				err := db.DB.Update(func(txn *badger.Txn) (e error) {
					_, err := db.IncrBy(db.GetAddressKey(tx.Address, db.KEY_BALANCE), tx.Value, false, txn)
					if err != nil {
						logs.Log.Errorf("Could not update account balance: %v", err)
						return errors.New("Could not update account balance!")
					}
					if tx.Value < 0 {
						err := db.Put(db.GetAddressKey(tx.Address, db.KEY_SPENT), true, nil, txn)
						if err != nil {
							logs.Log.Errorf("Could not update account spent status: %v", err)
							return errors.New("Could not update account spent status!")
						}
					}
					return nil
				})
				if err != nil {
					logs.Log.Errorf("Could not apply tx Value: %v", err)
					return errors.New("Could not apply Tx value!")
				}
			}
			x = x + 1
			if x % 10000 == 0 {
				logs.Log.Debug("Progress", x)
			}
		}
		return nil
	})
}