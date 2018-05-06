package tangle

import (
	"db"
	"github.com/dgraph-io/badger"
	"time"
)
const CONFIRM_CHECK_INTERVAL = time.Duration(10) * time.Second

func confirmOnLoad() {
	go startConfirmThread()
}

func startConfirmThread() {
	db.Locker.Lock()
	_ = db.DB.Update(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte{db.KEY_EVENT_CONFIRMATION_PENDING}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := it.Item().Key()
			confirm(key, txn)
		}
		return nil
	})
	db.Locker.Unlock()
	time.Sleep(CONFIRM_CHECK_INTERVAL)
	go startConfirmThread()
}

func confirm (key []byte, txn *badger.Txn) {
	// TODO: save milestone index in KEY_CONFIRMED and KEY_PENDING_CONFIRMED?
	// This way it would be easier to snapshot by milestone, not time..
	// But then, what to do with unconfirmed TXs?
	timestamp, err := db.GetInt(db.AsKey(key, db.KEY_TIMESTAMP), txn)
	value, err2 := db.GetInt64(db.AsKey(key, db.KEY_VALUE), txn)
	address, err3 := db.GetBytes(db.AsKey(key, db.KEY_ADDRESS_HASH), txn)
	relation, err4 := db.GetBytes(db.AsKey(key, db.KEY_RELATION), txn)
	if err != nil || err2 != nil || err3 != nil || err4 != nil {
		// Clearly missing transaction parts
		//log.Println("WHOOPS", len(key), err, err2, err3, err4)
		//panic("TX parts missing for confirmation!")
		return
	}
	db.Put(db.AsKey(key, db.KEY_CONFIRMED), timestamp, nil, txn)
	db.Remove(db.AsKey(key, db.KEY_EVENT_CONFIRMATION_PENDING), txn)
	if value > 0 {
		_, err := db.IncrBy(db.AsKey(address, db.KEY_BALANCE), value, true, txn)
		if err != nil {
			panic("Could not update account balance!")
		}
	}
	confirmChild(relation[:16], txn)
	confirmChild(relation[16:], txn)
}

func confirmChild (key []byte, txn *badger.Txn) {
	if db.Has(db.AsKey(key, db.KEY_CONFIRMED), txn) { return }
	_, err := db.GetBytes(db.AsKey(key, db.KEY_HASH), txn)
	if err == nil {
		db.Put(db.AsKey(key, db.KEY_EVENT_CONFIRMATION_PENDING), "", nil, txn)
		confirm(key, txn)
	} else {
		db.Put(db.AsKey(key, db.KEY_PENDING_CONFIRMED), int(time.Now().Unix()), nil, txn)
	}

}
