package snapshot

import (
	"db"
	"github.com/dgraph-io/badger"
	"logs"
	"transaction"
	"bytes"
	"encoding/gob"
	"convert"
)

func trimTXRunner () {
	logs.Log.Debug("Loading trimmable TXs", len(edgeTransactions))
	db.DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte{db.KEY_EDGE}
		total := 0
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			total++
			hashKey := db.AsKey(it.Item().Key(), db.KEY_HASH)
			logs.Log.Debug("LOADING", total, hashKey)
			edgeTransactions <- &hashKey
		}
		return nil
	})
	logs.Log.Debug("Loaded trimmable TXs", len(edgeTransactions))
	for hashKey := range edgeTransactions {
		logs.Log.Debug("TRIMMING", hashKey, len(edgeTransactions))
		trimTX(*hashKey)
	}
}

/*
Removes all data from the database that is before a given timestamp
 */
func trimData (timestamp int64) error {
	return db.DB.Update(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte{db.KEY_TIMESTAMP}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
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
						err := db.Put(db.AsKey(k, db.KEY_EDGE), true, nil, nil)
						hashKey := db.AsKey(k, db.KEY_HASH)
						edgeTransactions <- &hashKey
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
}

func trimTX (hashKey []byte) error {
	key := db.AsKey(hashKey, db.KEY_BYTES)
	txBytes, err := db.GetBytes(key, nil)
	var tx *transaction.FastTX
	if err == nil {
		trits := convert.BytesToTrits(txBytes)[:8019]
		tx = transaction.TritsToTX(&trits, txBytes)
	}
	return db.DB.Update(func(txn *badger.Txn) error {
		db.Remove(hashKey, txn)
		db.Remove(db.AsKey(hashKey, db.KEY_EDGE), txn)
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
