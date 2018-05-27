package tangle

import (
	"db"
	"github.com/dgraph-io/badger"
	"time"
	"logs"
	"github.com/pkg/errors"
	"bytes"
	"snapshot"
	"convert"
)
const CONFIRM_CHECK_INTERVAL = time.Duration(500) * time.Millisecond

func confirmOnLoad() {
	logs.Log.Info("Starting confirmation thread")
	go startConfirmThread()
}

func startConfirmThread() {
	for {
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
		time.Sleep(CONFIRM_CHECK_INTERVAL)
	}
}

func confirm (key []byte, txn *badger.Txn) error {
	db.Remove(db.AsKey(key, db.KEY_EVENT_CONFIRMATION_PENDING), txn)

	_, confirmedError := txn.Get(db.AsKey(key, db.KEY_CONFIRMED))
	if confirmedError == nil { return nil }

	timestamp, err := db.GetInt(db.AsKey(key, db.KEY_TIMESTAMP), txn)
	value, err2 := db.GetInt64(db.AsKey(key, db.KEY_VALUE), txn)
	address, err3 := db.GetBytes(db.AsKey(key, db.KEY_ADDRESS_HASH), txn)

	// Non-zero behind the snapshot horizon.
	if err == nil && err2 == nil && value == 0 && timestamp != 0 && !snapshot.IsNewerThanSnapshot(timestamp, txn){
		logs.Log.Warning("NOT newer!", timestamp, snapshot.GetSnapshotTimestamp(txn), convert.BytesToTrytes(address)[:81])
		return nil
	}

	relation, err4 := db.GetBytes(db.AsKey(key, db.KEY_RELATION), txn)
	if err != nil || err2 != nil || err3 != nil || err4 != nil {
		// Imminent database inconsistency: Warn!
		logs.Log.Errorf("Value-TX missing for confirmation. Probably snapshotted. DB inconsistency imminent!")
		return errors.New("TX parts missing for confirmation!")
	}

	err = db.Put(db.AsKey(key, db.KEY_CONFIRMED), timestamp, nil, txn)

	addressHash := db.GetByteKey(address, db.KEY_BALANCE)

	if err != nil {
		logs.Log.Errorf("Could not save confirmation status!", err)
		return errors.New("Could not save confirmation status!")
	}

	if value != 0 {
		_, err := db.IncrBy(addressHash, value, false, txn)
		if err != nil {
			logs.Log.Errorf("Could not update account balance: %v", err)
			return errors.New("Could not update account balance!")
		}
		if value < 0 {
			err := db.Put(db.AsKey(addressHash, db.KEY_SPENT), true, nil, txn)
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
	return nil
}

func confirmChild (key []byte, txn *badger.Txn) error {
	if bytes.Equal(key, tipHashKey) { return nil }
	if db.Has(db.AsKey(key, db.KEY_CONFIRMED), txn) { return nil }
	_, err := db.GetBytes(db.AsKey(key, db.KEY_HASH), txn)
	if err == nil {
		err = db.Put(db.AsKey(key, db.KEY_EVENT_CONFIRMATION_PENDING), "", nil, txn)
		if err != nil {
			logs.Log.Errorf("Could not save child confirm status: %v", err)
			return errors.New("Could not save child confirm status!")
		}
		//err = confirm(key, txn)
		// If err -> return err
	} else {
		err = db.Put(db.AsKey(key, db.KEY_PENDING_CONFIRMED), int(time.Now().Unix()), nil, txn)
		if err != nil {
			logs.Log.Errorf("Could not save child pending confirm status: %v", err)
			return errors.New("Could not save child pending confirm status!")
		}
	}
	return nil
}
