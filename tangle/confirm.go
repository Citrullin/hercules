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

const CONFIRM_CHECK_INTERVAL = time.Duration(500) * time.Microsecond
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
	go startConfirmThread()
}

func loadPendingConfirmations() {
	logs.Log.Debug("to load:", db.Count(db.KEY_EVENT_CONFIRMATION_PENDING))
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
	for pendingConfirmation := range confirmQueue {
		db.Locker.Lock()
		db.Locker.Unlock()
		err := db.DB.Update(func(txn *badger.Txn) error {
			return confirm(pendingConfirmation.key, txn)
		})
		if err != nil {
			go func () {
				time.Sleep(time.Second)
				confirmQueue <- pendingConfirmation
			}()
		}
		if snapshot.InProgress {
			time.Sleep(time.Second)
		} else {
			time.Sleep(CONFIRM_CHECK_INTERVAL)
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

func confirm(key []byte, txn *badger.Txn) error {
	if !addConfirmInProgress(key) {
		return nil
	}
	defer removeConfirmInProgress(key)

	db.Remove(db.AsKey(key, db.KEY_EVENT_CONFIRMATION_PENDING), txn)

	if db.Has(db.AsKey(key, db.KEY_CONFIRMED), txn) {
		return nil
	}

	data, err := db.GetBytes(db.AsKey(key, db.KEY_BYTES), txn)
	if err != nil {
		// Imminent database inconsistency: Warn!
		// logs.Log.Error("TX missing for confirmation. Probably snapshotted. DB inconsistency imminent!", key)
		return errors.New("TX  missing for confirmation!")
	}
	trits := convert.BytesToTrits(data)[:8019]
	var tx = transaction.TritsToFastTX(&trits, data)

	if db.Has(db.AsKey(key, db.KEY_EVENT_TRIM_PENDING), txn) && !bytes.Equal(tx.Address, COO_ADDRESS_BYTES) {
		logs.Log.Debug("TX pending for trim, skipping",
			tx.Timestamp, snapshot.GetSnapshotTimestamp(txn), convert.BytesToTrytes(tx.Address)[:81])
		if false && tx.Value != 0 {
			logs.Log.Errorf("TX with value %v skipped because of a trim - DB inconsistency imminent", tx.Value)
			return errors.New("Value TX confirmation behind snapshot horizon!")
		}
		return nil
	}

	err = db.Put(db.AsKey(key, db.KEY_CONFIRMED), tx.Timestamp, nil, txn)

	if err != nil {
		logs.Log.Errorf("Could not save confirmation status!", err)
		return errors.New("Could not save confirmation status!")
	}

	if tx.Value != 0 {
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
	}

	err = confirmChild(db.GetByteKey(tx.TrunkTransaction, db.KEY_HASH), txn)
	if err != nil {
		return err
	}
	err = confirmChild(db.GetByteKey(tx.BranchTransaction, db.KEY_HASH), txn)
	if err != nil {
		return err
	}
	totalConfirmations++
	return nil
}

func confirmChild(key []byte, txn *badger.Txn) error {
	if bytes.Equal(key, tipHashKey) {
		return nil
	}
	if db.Has(db.AsKey(key, db.KEY_CONFIRMED), txn) {
		return nil
	}
	timestamp, err := db.GetInt(db.AsKey(key, db.KEY_TIMESTAMP), txn)
	if err == nil {
		err = addPendingConfirmation(db.AsKey(key, db.KEY_EVENT_CONFIRMATION_PENDING), timestamp, txn)
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

func addConfirmInProgress (key []byte) bool {
	confirmLocker.Lock()
	defer confirmLocker.Unlock()

	k := string(key)
	_, ok := confirmsInProgress[k]
	if ok {
		return false
	}

	confirmsInProgress[k] = true
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