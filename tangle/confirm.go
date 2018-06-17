package tangle

import (
	"bytes"
	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
	"gitlab.com/semkodev/hercules/convert"
	"gitlab.com/semkodev/hercules/db"
	"gitlab.com/semkodev/hercules/logs"
	"gitlab.com/semkodev/hercules/snapshot"
	"time"
)

const CONFIRM_CHECK_INTERVAL = time.Duration(500) * time.Millisecond

func confirmOnLoad() {
	logs.Log.Info("Starting confirmation thread")
	go startConfirmThread()
}

func startConfirmThread() {
	for {
		if snapshot.InProgress {
			time.Sleep(time.Duration(1) * time.Second)
			continue
		}
		db.Locker.Lock()
		db.Locker.Unlock()
		_ = db.DB.View(func(txn *badger.Txn) error {
			opts := badger.DefaultIteratorOptions
			it := txn.NewIterator(opts)
			defer it.Close()
			prefix := []byte{db.KEY_EVENT_CONFIRMATION_PENDING}
			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				key := it.Item().Key()
				_ = db.DB.Update(func(txn *badger.Txn) error {
					return confirm(key, txn)
				})
			}
			return nil
		})
		/*/ Check unknown confirmed
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
		})*/
		time.Sleep(CONFIRM_CHECK_INTERVAL)
	}
}

func confirm(key []byte, txn *badger.Txn) error {
	db.Remove(db.AsKey(key, db.KEY_EVENT_CONFIRMATION_PENDING), txn)

	if db.Has(db.AsKey(key, db.KEY_CONFIRMED), txn) {
		return nil
	}

	timestamp, err := db.GetInt(db.AsKey(key, db.KEY_TIMESTAMP), txn)
	value, err2 := db.GetInt64(db.AsKey(key, db.KEY_VALUE), txn)
	address, err3 := db.GetBytes(db.AsKey(key, db.KEY_ADDRESS_HASH), txn)

	if db.Has(db.AsKey(key, db.KEY_EVENT_TRIM_PENDING), txn) && !bytes.Equal(address, COO_ADDRESS_BYTES) {
		logs.Log.Debug("TX pending for trim, skipping",
			timestamp, snapshot.GetSnapshotTimestamp(txn), convert.BytesToTrytes(address)[:81])
		if false && value != 0 {
			logs.Log.Errorf("TX with value %v skipped because of a trim - DB inconsistency imminent", value)
			return errors.New("Value TX confirmation behind snapshot horizon!")
		}
		return nil
	}

	relation, err4 := db.GetBytes(db.AsKey(key, db.KEY_RELATION), txn)
	if err != nil || err2 != nil || err3 != nil || err4 != nil {
		// Imminent database inconsistency: Warn!
		logs.Log.Error("TX missing for confirmation. Probably snapshotted. DB inconsistency imminent!", key)
		return errors.New("TX parts missing for confirmation!")
	}

	err = db.Put(db.AsKey(key, db.KEY_CONFIRMED), timestamp, nil, txn)

	if err != nil {
		logs.Log.Errorf("Could not save confirmation status!", err)
		return errors.New("Could not save confirmation status!")
	}

	// TODO: if value!= zero and keyindex is one: add bundle validation task
	// Add value only when bundle is consistent.
	if value != 0 {
		_, err := db.IncrBy(db.GetAddressKey(address, db.KEY_BALANCE), value, false, txn)
		if err != nil {
			logs.Log.Errorf("Could not update account balance: %v", err)
			return errors.New("Could not update account balance!")
		}
		if value < 0 {
			err := db.Put(db.GetAddressKey(address, db.KEY_SPENT), true, nil, txn)
			if err != nil {
				logs.Log.Errorf("Could not update account spent status: %v", err)
				return errors.New("Could not update account spent status!")
			}
		}
	}

	err = confirmChild(relation[:16], txn)
	if err != nil {
		return err
	}
	err2 = confirmChild(relation[16:], txn)
	if err2 != nil {
		return err2
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
		err = db.Put(db.AsKey(key, db.KEY_EVENT_CONFIRMATION_PENDING), timestamp, nil, txn)
		if err != nil {
			logs.Log.Errorf("Could not save child confirm status: %v", err)
			return errors.New("Could not save child confirm status!")
		}
	} else if !db.Has(db.AsKey(key, db.KEY_EDGE), txn) {
		err = db.Put(db.AsKey(key, db.KEY_PENDING_CONFIRMED), int(time.Now().Unix()), nil, txn)
		if err != nil {
			logs.Log.Errorf("Could not save child pending confirm status: %v", err)
			return errors.New("Could not save child pending confirm status!")
		}
	}
	return nil
}

// TODO: add rescanner!
