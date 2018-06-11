package snapshot

import (
	"bytes"
	"encoding/gob"
	"github.com/dgraph-io/badger"
	"gitlab.com/semkodev/hercules.go/db"
	"gitlab.com/semkodev/hercules.go/logs"
	"gitlab.com/semkodev/hercules.go/convert"
	"gitlab.com/semkodev/hercules.go/transaction"
)

func trimTXRunner () {
	logs.Log.Debug("Loading trimmable TXs", len(edgeTransactions))
	db.DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte{db.KEY_EVENT_TRIM_PENDING}
		total := 0
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			total++
			hashKey := db.AsKey(it.Item().Key(), db.KEY_HASH)
			edgeTransactions <- &hashKey
		}
		return nil
	})
	logs.Log.Debug("Loaded trimmable TXs", len(edgeTransactions))
	for hashKey := range edgeTransactions {
		db.Locker.Lock()
		db.Locker.Unlock()
		trimTX(*hashKey)
	}
}

/*
Removes all data from the database that is before a given timestamp
 */
 // TODO: (OPT) decrease transactions counter? What about confirmed? Not first priority now.
 // The counter can be treated as incremental counter of all TXs known plus received since start of the node.
func trimData (timestamp int64) error {
	var txs [][]byte
	var total = 0
	var found = 0

	err := db.DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte{db.KEY_TIMESTAMP}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			total++
			item := it.Item()
			k := item.Key()
			v, err := item.Value()
			if err == nil {
				var txTimestamp = 0
				buf := bytes.NewBuffer(v)
				dec := gob.NewDecoder(buf)
				err := dec.Decode(&txTimestamp)
				if err == nil {
					if int64(txTimestamp) <= timestamp {
						key := db.AsKey(k, db.KEY_EVENT_TRIM_PENDING)
						if !db.Has(key, txn) {
							txs = append(txs, key)
							found++
						}
						if err != nil {
							logs.Log.Error("Could not save pending edge transaction!")
							return err
						}
					}
				} else {
					logs.Log.Error("Could not parse a TX timestamp value!")
					return err
				}
			} else {
				logs.Log.Error("Could not get a TX timestamp value!")
				return err
			}
		}
		return nil
	})
	if err != nil { return err }

	logs.Log.Infof("Scheduling to trim %v transactions", found)
	txn := db.DB.NewTransaction(true)
	for _, k := range txs {
		err := db.Put(k, true, nil, txn)
		if err != nil {
			if err == badger.ErrTxnTooBig {
				_ = txn.Commit(func(e error) {})
				txn = db.DB.NewTransaction(true)
				err := db.Put(k, true, nil, txn)
				if err != nil { return err }
			} else {
				return err
			}
		}
		hashKey := db.AsKey(k, db.KEY_HASH)
		edgeTransactions <- &hashKey
	}
	_ = txn.Commit(func(e error) {})

	return nil
}

func trimTX (hashKey []byte) error {
	key := db.AsKey(hashKey, db.KEY_BYTES)
	txBytes, err := db.GetBytes(key, nil)
	var tx *transaction.FastTX
	if err == nil {
		trits := convert.BytesToTrits(txBytes)[:8019]
		tx = transaction.TritsToTX(&trits, txBytes)
	}
	//logs.Log.Debug("TRIMMING", hashKey)
	return db.DB.Update(func(txn *badger.Txn) error {
		db.Remove(hashKey, txn)
		db.Remove(db.AsKey(hashKey, db.KEY_EVENT_TRIM_PENDING), txn)
		db.Remove(db.AsKey(hashKey, db.KEY_EVENT_CONFIRMATION_PENDING), txn)
		db.Remove(db.AsKey(hashKey, db.KEY_TIMESTAMP), txn)
		db.Remove(db.AsKey(hashKey, db.KEY_BYTES), txn)
		db.Remove(db.AsKey(hashKey, db.KEY_VALUE), txn)
		db.Remove(db.AsKey(hashKey, db.KEY_ADDRESS_HASH), txn)
		db.Remove(db.AsKey(hashKey, db.KEY_RELATION), txn)
		db.Remove(db.AsKey(hashKey, db.KEY_CONFIRMED), txn)
		db.Remove(db.AsKey(hashKey, db.KEY_MILESTONE), txn)
		db.Remove(db.AsKey(hashKey, db.KEY_RELATION), txn)
		if tx != nil {
			db.Remove(append(db.GetByteKey(tx.TrunkTransaction, db.KEY_APPROVEE), hashKey...), txn)
			db.Remove(append(db.GetByteKey(tx.BranchTransaction, db.KEY_APPROVEE), hashKey...), txn)
			db.Remove(append(db.GetByteKey(tx.Bundle, db.KEY_BUNDLE), hashKey...), txn)
			db.Remove(append(db.GetByteKey(tx.Tag, db.KEY_TAG), hashKey...), txn)
			db.Remove(append(db.GetByteKey(tx.Address, db.KEY_ADDRESS), hashKey...), txn)

		}
		return nil
	})
}
