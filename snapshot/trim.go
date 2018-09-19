package snapshot

import (
	"gitlab.com/semkodev/hercules/config"
	"gitlab.com/semkodev/hercules/convert"
	"gitlab.com/semkodev/hercules/db"
	"gitlab.com/semkodev/hercules/db/coding"
	"gitlab.com/semkodev/hercules/db/ns"
	"gitlab.com/semkodev/hercules/logs"
	"gitlab.com/semkodev/hercules/transaction"
)

func trimTXRunner() {
	edgeTransactionsWaitGroup.Add(1)
	defer edgeTransactionsWaitGroup.Done()

	if config.AppConfig.GetBool("snapshots.keep") {
		logs.Log.Notice("The trimmed transactions after the snapshots will be kept in the database.")
		return
	}

	logs.Log.Debug("Loading trimmable TXs", len(edgeTransactions))

	db.Singleton.View(func(dbTx db.Transaction) error {
		ns.ForNamespace(dbTx, ns.NamespaceEventTrimPending, false, func(key, _ []byte) (bool, error) {
			hashKey := ns.Key(key, ns.NamespaceHash)
			edgeTransactions <- &hashKey
			return true, nil
		})
		return nil
	})

	logs.Log.Debug("Loaded trimmable TXs", len(edgeTransactions))

	for {
		select {
		case <-edgeTransactionsQueueQuit:
			return

		case hashKey := <-edgeTransactions:
			SnapshotWaitGroup.Wait()
			trimTX(*hashKey)
		}
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

	err := db.Singleton.View(func(dbTx db.Transaction) error {
		err := coding.ForPrefixInt64(dbTx, ns.Prefix(ns.NamespaceTimestamp), false, func(k []byte, txTimestamp int64) (bool, error) {
			// TODO: since the milestone timestamps are often zero, it might be a good idea to keep them..?
			// Theoretically, they are not needed any longer. :-/
			if txTimestamp <= timestamp {
				key := ns.Key(k, ns.NamespaceEventTrimPending)
				if !dbTx.HasKey(key) {
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
	dbTx := db.Singleton.NewTransaction(true)
	for _, k := range txs {
		if err := coding.PutBool(dbTx, k, true); err != nil {
			if err == db.ErrTransactionTooBig {
				_ = dbTx.Commit()
				dbTx = db.Singleton.NewTransaction(true)
				if err := coding.PutBool(dbTx, k, true); err != nil {
					return err
				}
			} else {
				return err
			}
		}
		hashKey := ns.Key(k, ns.NamespaceHash)
		edgeTransactions <- &hashKey
	}
	_ = dbTx.Commit()

	return nil
}

func trimTX(hashKey []byte) error {
	key := ns.Key(hashKey, ns.NamespaceBytes)
	tBytes, err := db.Singleton.GetBytes(key)
	var tx *transaction.FastTX
	if err == nil {
		trits := convert.BytesToTrits(tBytes)[:8019]
		tx = transaction.TritsToTX(&trits, tBytes)
	}
	//logs.Log.Debug("TRIMMING", hashKey)
	return db.Singleton.Update(func(dbTx db.Transaction) error {
		dbTx.Remove(hashKey)
		dbTx.Remove(ns.Key(hashKey, ns.NamespaceEventTrimPending))
		dbTx.Remove(ns.Key(hashKey, ns.NamespaceEventConfirmationPending))
		dbTx.Remove(ns.Key(hashKey, ns.NamespaceTimestamp))
		dbTx.Remove(ns.Key(hashKey, ns.NamespaceBytes))
		dbTx.Remove(ns.Key(hashKey, ns.NamespaceValue))
		dbTx.Remove(ns.Key(hashKey, ns.NamespaceAddressHash))
		dbTx.Remove(ns.Key(hashKey, ns.NamespaceRelation))
		dbTx.Remove(ns.Key(hashKey, ns.NamespaceConfirmed))
		dbTx.Remove(ns.Key(hashKey, ns.NamespaceMilestone))
		dbTx.Remove(ns.Key(hashKey, ns.NamespaceRelation))
		dbTx.Remove(ns.Key(hashKey, ns.NamespaceGTTA))
		if dbTx != nil {
			dbTx.Remove(append(ns.HashKey(tx.TrunkTransaction, ns.NamespaceApprovee), hashKey...))
			dbTx.Remove(append(ns.HashKey(tx.BranchTransaction, ns.NamespaceApprovee), hashKey...))
			dbTx.Remove(append(ns.HashKey(tx.Bundle, ns.NamespaceBundle), hashKey...))
			dbTx.Remove(append(ns.HashKey(tx.Tag, ns.NamespaceTag), hashKey...))
			dbTx.Remove(append(ns.HashKey(tx.Address, ns.NamespaceAddress), hashKey...))
		}
		return nil
	})
}
