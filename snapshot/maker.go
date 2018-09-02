package snapshot

import (
	"bytes"

	"../convert"
	"../db"
	"../db/coding"
	"../logs"
	"../transaction"
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
	InProgress = true
	defer func() {
		InProgress = false
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

	err := db.Singleton.Update(func(tx db.Transaction) error {
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
		lockedTimestamp := GetSnapshotLock(tx)
		if lockedTimestamp > timestamp {
			logs.Log.Warningf("There is a snapshot pending (%v), skipping current (%v)!", lockedTimestamp, timestamp)
			return errors.New("pending snapshot, skipping current one")
		}

		Lock(timestamp, "", tx)

		logs.Log.Debug("Collecting all value bundles before the snapshot horizon...")
		err := coding.ForPrefixInt64(tx, []byte{db.KEY_TIMESTAMP}, false, func(k []byte, txTimestamp int64) (bool, error) {
			if txTimestamp > timestamp {
				return true, nil
			}

			key := db.AsKey(k, db.KEY_CONFIRMED)
			if tx.HasKey(key) && !tx.HasKey(db.AsKey(key, db.KEY_EVENT_TRIM_PENDING)) {
				value, err := coding.GetInt64(tx, db.AsKey(key, db.KEY_VALUE))
				if err != nil || value == 0 {
					return true, nil
				}

				txBytes, err := tx.GetBytes(db.AsKey(key, db.KEY_BYTES))
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
			bundleTxs, snaps, bundleKeep, err := loadAllFromBundle(bundleHash, timestamp, tx)
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
		err := db.Singleton.Update(func(tx db.Transaction) error {
			// First: update snapshot balances
			address, err := tx.GetBytes(db.AsKey(kv.key, db.KEY_ADDRESS_HASH))
			if err != nil {
				return err
			}

			_, err = tx.IncrementBy(db.GetAddressKey(address, db.KEY_SNAPSHOT_BALANCE), kv.value, false)
			if err != nil {
				return err
			}

			// Update spents:
			if kv.value < 0 {
				err := coding.PutBool(tx, db.GetAddressKey(address, db.KEY_SNAPSHOT_SPENT), true)
				if err != nil {
					return err
				}
			}

			// Create trimming event:
			trimKey = db.AsKey(kv.key, db.KEY_EVENT_TRIM_PENDING)
			return coding.PutBool(tx, trimKey, true)
		})
		if err != nil {
			return err
		}
		edgeTransactions <- &trimKey
	}

	db.Singleton.RemoveKeysFromCategoryBefore(db.KEY_PENDING_BUNDLE, int64(timestamp))
	for _, key := range toKeepBundle {
		err := coding.PutInt64(db.Singleton, key, timestamp)
		if err != nil {
			return err
		}
	}

	db.Singleton.RemoveKeysFromCategoryBefore(db.KEY_SNAPSHOTTED, int64(timestamp))
	for _, key := range snapshotted {
		err := coding.PutInt64(db.Singleton, key, timestamp)
		if err != nil {
			return err
		}
	}

	db.Singleton.RemoveKeysFromCategoryBefore(db.KEY_EDGE, int64(timestamp))

	if checkDatabaseSnapshot() {
		logs.Log.Debug("Scheduling transaction trimming")
		trimData(int64(timestamp))
		logs.Log.Notice("Snapshot applied. Critical moment over.")
		return db.Singleton.Update(func(tx db.Transaction) error {
			err := SetSnapshotTimestamp(timestamp, tx)
			if err != nil {
				return err
			}

			err = Unlock(tx)
			if err != nil {
				return err
			}
			tx.RemoveKeyCategory(db.KEY_EDGE)
			path := config.GetString("snapshots.path")
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

func loadAllFromBundle(bundleHash []byte, timestamp int64, tx db.Transaction) ([]KeyValue, [][]byte, []byte, error) {
	var (
		totalValue  int64 = 0
		txs         []KeyValue
		snapshotted [][]byte
		nonZero     = false
	)

	prefix := db.GetByteKey(bundleHash, db.KEY_BUNDLE)
	err := tx.ForPrefix(prefix, false, func(k, _ []byte) (bool, error) {
		key := make([]byte, 16)
		copy(key, k[16:])

		// Filter out unconfirmed reattachments, snapshotted txs or future snapshotted txs:
		if !canBeSnapshotted(key, tx) {
			return true, nil
		}

		txTimestamp, err := coding.GetInt64(tx, db.AsKey(key, db.KEY_TIMESTAMP))
		if err != nil {
			return false, err
		}
		if txTimestamp > timestamp {
			snapshotted = append(snapshotted, db.AsKey(prefix, db.KEY_SNAPSHOTTED))
		}

		valueKey := db.AsKey(key, db.KEY_VALUE)
		value, err := coding.GetInt64(tx, valueKey)
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
		// return nil, nil, db.AsKey(prefix, db.KEY_PENDING_BUNDLE), nil
		/**/
		logs.Log.Errorf("A bundle is incomplete (non-zero sum). "+
			"The database is probably inconsistent, not in sync or the timestamp is too early! %v", convert.BytesToTrytes(bundleHash)[:81])
		return nil, nil, nil, errors.New("A bundle is incomplete (non-zero sum). The database is probably inconsistent, not in sync or timestamp is too early!")
		/**/
	}
	return txs, snapshotted, nil, nil
}
