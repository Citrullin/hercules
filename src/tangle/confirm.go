package tangle

import (
	"db"
	"github.com/dgraph-io/badger"
	"time"
	"logs"
	"github.com/pkg/errors"
)
const CONFIRM_CHECK_INTERVAL = time.Duration(700) * time.Millisecond

func confirmOnLoad() {
	go startConfirmThread()
}

func startConfirmThread() {
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
	go startConfirmThread()
}

func confirm (key []byte, txn *badger.Txn) error {
	timestamp, err := db.GetInt(db.AsKey(key, db.KEY_TIMESTAMP), txn)
	value, err2 := db.GetInt64(db.AsKey(key, db.KEY_VALUE), txn)
	address, err3 := db.GetBytes(db.AsKey(key, db.KEY_ADDRESS_HASH), txn)
	relation, err4 := db.GetBytes(db.AsKey(key, db.KEY_RELATION), txn)
	if err != nil || err2 != nil || err3 != nil || err4 != nil {
		// Clearly missing transaction parts
		logs.Log.Warningf("TX parts missing for confirmation!")
		logs.Log.Errorf(" -> ERRORS: %v, %v, %v, %v", err, err2, err3, err4)
		return errors.New("TX parts missing for confirmation!")
	}
	err = db.Put(db.AsKey(key, db.KEY_CONFIRMED), timestamp, nil, txn)
	err2 = db.Remove(db.AsKey(key, db.KEY_EVENT_CONFIRMATION_PENDING), txn)

	if err != nil || err2 != nil {
		logs.Log.Errorf("Could not save confirmation status!")
		logs.Log.Errorf(" -> ERRORS: %v, %v", err, err2)
		return errors.New("Could not save confirmation status!")
	}

	if value != 0 {
		_, err := db.IncrBy(db.AsKey(address, db.KEY_BALANCE), value, false, txn)
		if err != nil {
			logs.Log.Errorf("Could not update account balance: %v", err)
			return errors.New("Could not update account balance!")
		}
		if value < 0 {
			err := db.Put(db.AsKey(address, db.KEY_SPENT), true, nil, txn)
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
