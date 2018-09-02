package snapshot

import (
	"bytes"
	"encoding/gob"
	"time"

	"../convert"
	"../db"
	"../db/coding"
	"../logs"
	"../transaction"
)

func trimTXRunner() {
	if config.GetBool("snapshots.keep") {
		logs.Log.Notice("The trimmed transactions after the snapshots will be kept in the database.")
		return
	}
	logs.Log.Debug("Loading trimmable TXs", len(edgeTransactions))
	db.Singleton.View(func(tx db.Transaction) error {
		tx.ForPrefix([]byte{db.KEY_EVENT_TRIM_PENDING}, false, func(key, _ []byte) (bool, error) {
			hashKey := db.AsKey(key, db.KEY_HASH)
			edgeTransactions <- &hashKey
			return true, nil
		})
		return nil
	})
	logs.Log.Debug("Loaded trimmable TXs", len(edgeTransactions))
	for hashKey := range edgeTransactions {
		db.Singleton.Lock()
		db.Singleton.Unlock() // should this be unlocked?
		if InProgress {
			time.Sleep(1 * time.Second)
			continue
		}
		trimTX(*hashKey)
	}
}

/*
Removes all data from the database that is before a given timestamp
*/
// TODO: (OPT) decrease transactions counter? What about confirmed? Not first priority now.
// The counter can be treated as incremental counter of all TXs known plus received since start of the node.
func trimData(timestamp int64) error {
	var txs [][]byte
	var found = 0

	err := db.Singleton.View(func(tx db.Transaction) error {
		err := tx.ForPrefix([]byte{db.KEY_TIMESTAMP}, true, func(k, v []byte) (bool, error) {
			var txTimestamp = 0
			if err := gob.NewDecoder(bytes.NewBuffer(v)).Decode(&txTimestamp); err != nil {
				logs.Log.Error("Could not parse a TX timestamp value!")
				return false, err
			}

			// TODO: since the milestone timestamps are often zero, it might be a good idea to keep them..?
			// Theoretically, they are not needed any longer. :-/
			if int64(txTimestamp) <= timestamp {
				key := db.AsKey(k, db.KEY_EVENT_TRIM_PENDING)
				if !tx.HasKey(key) {
					txs = append(txs, key)
					found++
				}
			}

			return true, nil
		})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	logs.Log.Infof("Scheduling to trim %v transactions", found)
	tx := db.Singleton.NewTransaction(true)
	for _, k := range txs {
		if err := coding.PutBool(tx, k, true); err != nil {
			if err == db.ErrTransactionTooBig {
				_ = tx.Commit()
				tx = db.Singleton.NewTransaction(true)
				if err := coding.PutBool(tx, k, true); err != nil {
					return err
				}
			} else {
				return err
			}
		}
		hashKey := db.AsKey(k, db.KEY_HASH)
		edgeTransactions <- &hashKey
	}
	_ = tx.Commit()

	return nil
}

func trimTX(hashKey []byte) error {
	key := db.AsKey(hashKey, db.KEY_BYTES)
	tBytes, err := db.Singleton.GetBytes(key)
	var t *transaction.FastTX
	if err == nil {
		trits := convert.BytesToTrits(tBytes)[:8019]
		t = transaction.TritsToTX(&trits, tBytes)
	}
	//logs.Log.Debug("TRIMMING", hashKey)
	return db.Singleton.Update(func(tx db.Transaction) error {
		tx.Remove(hashKey)
		tx.Remove(db.AsKey(hashKey, db.KEY_EVENT_TRIM_PENDING))
		tx.Remove(db.AsKey(hashKey, db.KEY_EVENT_CONFIRMATION_PENDING))
		tx.Remove(db.AsKey(hashKey, db.KEY_TIMESTAMP))
		tx.Remove(db.AsKey(hashKey, db.KEY_BYTES))
		tx.Remove(db.AsKey(hashKey, db.KEY_VALUE))
		tx.Remove(db.AsKey(hashKey, db.KEY_ADDRESS_HASH))
		tx.Remove(db.AsKey(hashKey, db.KEY_RELATION))
		tx.Remove(db.AsKey(hashKey, db.KEY_CONFIRMED))
		tx.Remove(db.AsKey(hashKey, db.KEY_MILESTONE))
		tx.Remove(db.AsKey(hashKey, db.KEY_RELATION))
		tx.Remove(db.AsKey(hashKey, db.KEY_GTTA))
		if tx != nil {
			tx.Remove(append(db.GetByteKey(t.TrunkTransaction, db.KEY_APPROVEE), hashKey...))
			tx.Remove(append(db.GetByteKey(t.BranchTransaction, db.KEY_APPROVEE), hashKey...))
			tx.Remove(append(db.GetByteKey(t.Bundle, db.KEY_BUNDLE), hashKey...))
			tx.Remove(append(db.GetByteKey(t.Tag, db.KEY_TAG), hashKey...))
			tx.Remove(append(db.GetByteKey(t.Address, db.KEY_ADDRESS), hashKey...))
		}
		return nil
	})
}
