package snapshot

import (
	"time"

	"../config"
	"../convert"
	"../db"
	"../db/coding"
	"../db/ns"
	"../logs"
	"../transaction"
)

func trimTXRunner() {
	if config.AppConfig.GetBool("snapshots.keep") {
		logs.Log.Notice("The trimmed transactions after the snapshots will be kept in the database.")
		return
	}
	logs.Log.Debug("Loading trimmable TXs", len(edgeTransactions))
	db.Singleton.View(func(tx db.Transaction) error {
		ns.ForNamespace(tx, ns.NamespaceEventTrimPending, false, func(key, _ []byte) (bool, error) {
			hashKey := ns.Key(key, ns.NamespaceHash)
			edgeTransactions <- &hashKey
			return true, nil
		})
		return nil
	})
	logs.Log.Debug("Loaded trimmable TXs", len(edgeTransactions))
	for hashKey := range edgeTransactions {
		db.Singleton.Lock()
		db.Singleton.Unlock() // should this be unlocked?
		if SnapshotInProgress {
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
		err := coding.ForPrefixInt64(tx, ns.Prefix(ns.NamespaceTimestamp), false, func(k []byte, txTimestamp int64) (bool, error) {
			// TODO: since the milestone timestamps are often zero, it might be a good idea to keep them..?
			// Theoretically, they are not needed any longer. :-/
			if txTimestamp <= timestamp {
				key := ns.Key(k, ns.NamespaceEventTrimPending)
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
		hashKey := ns.Key(k, ns.NamespaceHash)
		edgeTransactions <- &hashKey
	}
	_ = tx.Commit()

	return nil
}

func trimTX(hashKey []byte) error {
	key := ns.Key(hashKey, ns.NamespaceBytes)
	tBytes, err := db.Singleton.GetBytes(key)
	var t *transaction.FastTX
	if err == nil {
		trits := convert.BytesToTrits(tBytes)[:8019]
		t = transaction.TritsToTX(&trits, tBytes)
	}
	//logs.Log.Debug("TRIMMING", hashKey)
	return db.Singleton.Update(func(tx db.Transaction) error {
		tx.Remove(hashKey)
		tx.Remove(ns.Key(hashKey, ns.NamespaceEventTrimPending))
		tx.Remove(ns.Key(hashKey, ns.NamespaceEventConfirmationPending))
		tx.Remove(ns.Key(hashKey, ns.NamespaceTimestamp))
		tx.Remove(ns.Key(hashKey, ns.NamespaceBytes))
		tx.Remove(ns.Key(hashKey, ns.NamespaceValue))
		tx.Remove(ns.Key(hashKey, ns.NamespaceAddressHash))
		tx.Remove(ns.Key(hashKey, ns.NamespaceRelation))
		tx.Remove(ns.Key(hashKey, ns.NamespaceConfirmed))
		tx.Remove(ns.Key(hashKey, ns.NamespaceMilestone))
		tx.Remove(ns.Key(hashKey, ns.NamespaceRelation))
		tx.Remove(ns.Key(hashKey, ns.NamespaceGTTA))
		if tx != nil {
			tx.Remove(append(ns.HashKey(t.TrunkTransaction, ns.NamespaceApprovee), hashKey...))
			tx.Remove(append(ns.HashKey(t.BranchTransaction, ns.NamespaceApprovee), hashKey...))
			tx.Remove(append(ns.HashKey(t.Bundle, ns.NamespaceBundle), hashKey...))
			tx.Remove(append(ns.HashKey(t.Tag, ns.NamespaceTag), hashKey...))
			tx.Remove(append(ns.HashKey(t.Address, ns.NamespaceAddress), hashKey...))
		}
		return nil
	})
}
