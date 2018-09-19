package snapshot

import (
	"bytes"

	"gitlab.com/semkodev/hercules/config"
	"gitlab.com/semkodev/hercules/convert"
	"gitlab.com/semkodev/hercules/db"
	"gitlab.com/semkodev/hercules/db/coding"
	"gitlab.com/semkodev/hercules/db/ns"
	"gitlab.com/semkodev/hercules/logs"
	"gitlab.com/semkodev/hercules/transaction"
	"github.com/pkg/errors"
)

type KeyValue struct {
	key   []byte
	value int64
}

/*
Creates a snapshot on the current tangle database.
*/
func MakeSnapshot(timestamp int64, filename string) error {
	logs.Log.Infof("Making snapshot for Unix time %v...", timestamp)
	SnapshotInProgress = true
	SnapshotWaitGroup.Add(1)
	defer func() {
		SnapshotInProgress = false
		SnapshotWaitGroup.Done()
	}()

	var bundles [][]byte
	var txs []KeyValue
	var toKeepBundle [][]byte
	var snapshotted [][]byte

	contains := func(key []byte) bool {
		if bundles == nil {
			return false
		}
		for _, k := range bundles {
			if bytes.Equal(k, key) {
				return true
			}
		}
		return false
	}

	err := db.Singleton.Update(func(dbTx db.Transaction) error {
		if !IsEqualOrNewerThanSnapshot(timestamp, nil) {
			logs.Log.Infof("The given snapshot (%v) timestamp is older than the current one. Skipping", timestamp)
			return errors.New("given snapshot is older than current one")
		}
		if !IsSynchronized() {
			logs.Log.Warning("Tangle not fully synchronized - cannot create snapshot!")
			return errors.New("tangle not fully synchronized")
		}
		if !CanSnapshot(timestamp) {
			logs.Log.Warning("Pending confirmations behind the snapshot horizon - cannot create snapshot!")
			return errors.New("tangle not fully synchronized")
		}
		lockedTimestamp := GetSnapshotLock(dbTx)
		if lockedTimestamp > timestamp {
			logs.Log.Warningf("There is a snapshot pending (%v), skipping current (%v)!", lockedTimestamp, timestamp)
			return errors.New("pending snapshot, skipping current one")
		}

		Lock(timestamp, "", dbTx)

		logs.Log.Debug("Collecting all value bundles before the snapshot horizon...")
		err := coding.ForPrefixInt64(dbTx, ns.Prefix(ns.NamespaceTimestamp), false, func(k []byte, txTimestamp int64) (bool, error) {
			if txTimestamp > timestamp {
				return true, nil
			}

			key := ns.Key(k, ns.NamespaceConfirmed)
			if dbTx.HasKey(key) && !dbTx.HasKey(ns.Key(key, ns.NamespaceEventTrimPending)) {
				value, err := coding.GetInt64(dbTx, ns.Key(key, ns.NamespaceValue))
				if err != nil || value == 0 {
					return true, nil
				}

				txBytes, err := dbTx.GetBytes(ns.Key(key, ns.NamespaceBytes))
				if err != nil {
					return false, err
				}
				trits := convert.BytesToTrits(txBytes)[:8019]
				tx := transaction.TritsToFastTX(&trits, txBytes)
				if !contains(tx.Bundle) {
					bundles = append(bundles, tx.Bundle)
				}
			}

			return true, nil
		})
		if err != nil {
			return err
		}

		logs.Log.Debugf("Found %v value bundles. Collecting corresponding transactions...", len(bundles))
		for _, bundleHash := range bundles {
			bundleTxs, snaps, bundleKeep, err := loadAllFromBundle(bundleHash, timestamp, dbTx)
			if err != nil {
				return err
			}
			txs = append(txs, bundleTxs...)
			if bundleKeep != nil && len(bundleKeep) == 16 {
				logs.Log.Debug("Keeping bundle...", bundleKeep)
				toKeepBundle = append(toKeepBundle, bundleKeep)
			}
			if snaps != nil {
				snapshotted = append(snapshotted, snaps...)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	logs.Log.Debugf("Found %v value transactions. Applying to previous snapshot...", len(txs))
	logs.Log.Notice("Applying snapshot. Critical moment. Do not turn off your computer.")
	for _, kv := range txs {
		var trimKey []byte
		err := db.Singleton.Update(func(dbTx db.Transaction) error {
			// First: update snapshot balances
			address, err := dbTx.GetBytes(ns.Key(kv.key, ns.NamespaceAddressHash))
			if err != nil {
				return err
			}

			_, err = coding.IncrementInt64By(dbTx, ns.AddressKey(address, ns.NamespaceSnapshotBalance), kv.value, false)
			if err != nil {
				return err
			}

			// Update spents:
			if kv.value < 0 {
				err := coding.PutBool(dbTx, ns.AddressKey(address, ns.NamespaceSnapshotSpent), true)
				if err != nil {
					return err
				}
			}

			// Create trimming event:
			trimKey = ns.Key(kv.key, ns.NamespaceEventTrimPending)
			return coding.PutBool(dbTx, trimKey, true)
		})
		if err != nil {
			return err
		}
		edgeTransactions <- &trimKey
	}

	coding.RemoveKeysInCategoryWithInt64LowerEqual(db.Singleton, ns.NamespacePendingBundle, timestamp)
	for _, key := range toKeepBundle {
		err := coding.PutInt64(db.Singleton, key, timestamp)
		if err != nil {
			return err
		}
	}

	coding.RemoveKeysInCategoryWithInt64LowerEqual(db.Singleton, ns.NamespaceSnapshotted, timestamp)
	for _, key := range snapshotted {
		err := coding.PutInt64(db.Singleton, key, timestamp)
		if err != nil {
			return err
		}
	}

	coding.RemoveKeysInCategoryWithInt64LowerEqual(db.Singleton, ns.NamespaceEdge, timestamp)

	if checkDatabaseSnapshot() {
		logs.Log.Debug("Scheduling transaction trimming")
		trimData(int64(timestamp))
		logs.Log.Notice("Snapshot applied. Critical moment over.")
		return db.Singleton.Update(func(dbTx db.Transaction) error {
			err := SetSnapshotTimestamp(timestamp, dbTx)
			if err != nil {
				return err
			}

			err = Unlock(dbTx)
			if err != nil {
				return err
			}
			ns.Remove(dbTx, ns.NamespaceEdge)
			path := config.AppConfig.GetString("snapshots.path")
			err = SaveSnapshot(path, timestamp, filename)
			if err != nil {
				return err
			}
			logs.Log.Info("Snapshot finished and saved in", path)
			return nil
		})
	}
	return errors.New("failed database snapshot integrity check")
}

func loadAllFromBundle(bundleHash []byte, timestamp int64, dbTx db.Transaction) ([]KeyValue, [][]byte, []byte, error) {
	var (
		totalValue  int64 = 0
		txs         []KeyValue
		snapshotted [][]byte
		nonZero     = false
	)

	prefix := ns.HashKey(bundleHash, ns.NamespaceBundle)
	err := dbTx.ForPrefix(prefix, false, func(k, _ []byte) (bool, error) {
		key := make([]byte, 16)
		copy(key, k[16:])

		// Filter out unconfirmed reattachments, snapshotted txs or future snapshotted txs:
		if !canBeSnapshotted(key, dbTx) {
			return true, nil
		}

		txTimestamp, err := coding.GetInt64(dbTx, ns.Key(key, ns.NamespaceTimestamp))
		if err != nil {
			return false, err
		}
		if txTimestamp > timestamp {
			snapshotted = append(snapshotted, ns.Key(prefix, ns.NamespaceSnapshotted))
		}

		valueKey := ns.Key(key, ns.NamespaceValue)
		value, err := coding.GetInt64(dbTx, valueKey)
		if err != nil {
			logs.Log.Errorf("Error reading value for %v", valueKey)
			return false, err
		}

		if value != 0 {
			totalValue += value
			txs = append(txs, KeyValue{valueKey, value})
			nonZero = true
		}

		return true, nil
	})
	if err != nil {
		return nil, nil, nil, err
	}

	// Probably debris from last snapshot. Has most probably to do with timestamps vs attachment timestamps
	if totalValue != 0 || !nonZero {
		// return nil, nil, ns.Key(prefix, ns.NamespacePendingBundle), nil
		/**/
		logs.Log.Errorf("A bundle is incomplete (non-zero sum). "+
			"The database is probably inconsistent, not in sync or the timestamp is too early! %v", convert.BytesToTrytes(bundleHash)[:81])
		return nil, nil, nil, errors.New("A bundle is incomplete (non-zero sum). The database is probably inconsistent, not in sync or timestamp is too early!")
		/**/
	}
	return txs, snapshotted, nil, nil
}
