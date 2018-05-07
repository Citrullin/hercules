package tangle

import (
	"time"
	"db"
	"github.com/dgraph-io/badger"
	"logs"
)

func periodicRequest() {
	ticker := time.NewTicker(pingInterval)
	for range ticker.C {
		db.Locker.Lock()
		db.Locker.Unlock()
		_ = db.DB.View(func(txn *badger.Txn) error {
			// Request pending
			outgoingQueue <- getMessage(nil, nil, false, txn)
			return nil
		})
	}
}

func periodicTipRequest() {
	ticker := time.NewTicker(tipPingInterval)
	for range ticker.C {
		db.Locker.Lock()
		db.Locker.Unlock()
		_ = db.DB.View(func(txn *badger.Txn) error {
			// Request tip
			outgoingQueue <- getMessage(nil, nil, true, txn)
			return nil
		})
	}
}

func requestIfMissing (hash []byte, addr string, txn *badger.Txn) (has bool, err error) {
	tx := txn
	has = true
	if txn == nil {
		tx = db.DB.NewTransaction(true)
		defer func () error {
			return tx.Commit(func(e error) {})
		}()
	}
	if !db.Has(db.GetByteKey(hash, db.KEY_HASH), tx) { //&& !db.Has(db.GetByteKey(hash, db.KEY_PENDING), tx) {
		err1 := db.Put(db.GetByteKey(hash, db.KEY_PENDING), int(time.Now().Unix()), nil, tx)
		err2 := db.Put(db.GetByteKey(hash, db.KEY_PENDING_HASH), hash, nil, tx)
		if err1 != nil {
			logs.Log.Error("Failed saving new TX request", err1)
			return has, err1
		} else if err2 != nil {
			logs.Log.Error("Failed saving new TX request", err2)
			return has, err2
		}
		message := getMessage(nil, hash, false, tx)
		message.Addr = addr
		outgoingQueue <- message
		has = false
	}
	return has, nil
}

