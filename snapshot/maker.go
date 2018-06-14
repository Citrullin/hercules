package snapshot

import (
	"bytes"
	"encoding/gob"
	"github.com/pkg/errors"
	"github.com/dgraph-io/badger"
	"gitlab.com/semkodev/hercules/db"
	"gitlab.com/semkodev/hercules/logs"
	"gitlab.com/semkodev/hercules/convert"
	"gitlab.com/semkodev/hercules/transaction"
)

type KeyValue struct {
	key []byte
	value int64
}

/*
Creates a snapshot on the current tangle database.
 */
func MakeSnapshot (timestamp int) error {
	logs.Log.Infof("Making snapshot for Unix time %v...", timestamp)
	InProgress = true
	db.CleanupLocker.Lock()
	defer func() {
		InProgress = false
		db.CleanupLocker.Unlock()
	}()

	var bundles [][]byte
	var txs []KeyValue
	var toKeepBundle [][]byte
	var snapshotted [][]byte

	contains := func(key []byte) bool {
		if bundles == nil { return false }
		for _, k := range bundles {
			if bytes.Equal(k, key) { return true }
		}
		return false
	}

	err := db.DB.Update(func(txn *badger.Txn) error {
		if !IsEqualOrNewerThanSnapshot(int(timestamp), nil) {
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
		lockedTimestamp := GetSnapshotLock(txn)
		if lockedTimestamp > timestamp {
			logs.Log.Warningf("There is a snapshot pending (%v), skipping current (%v)!", lockedTimestamp, timestamp)
			return errors.New("pending snapshot, skipping current one")
		}

		Lock(int(timestamp), "", nil)

		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte{db.KEY_TIMESTAMP}
		logs.Log.Debug("Collecting all value bundles before the snapshot horizon...")
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
					if txTimestamp <= timestamp {
						key := db.AsKey(k, db.KEY_CONFIRMED)
						if db.Has(key, txn) && !db.Has(db.AsKey(key, db.KEY_EVENT_TRIM_PENDING), txn){
							value, err := db.GetInt64(db.AsKey(key, db.KEY_VALUE), txn)
							if err == nil && value != 0 {
								txBytes, err := db.GetBytes(db.AsKey(key, db.KEY_BYTES), txn)
								if err != nil { return err }
								trits := convert.BytesToTrits(txBytes)[:8019]
								tx := transaction.TritsToFastTX(&trits, txBytes)
								if !contains(tx.Bundle) {
									bundles = append(bundles, tx.Bundle)
								}
							}
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
		logs.Log.Debugf("Found %v value bundles. Collecting corresponding transactions...", len(bundles))
		for _, bundleHash := range bundles {
			bundleTxs, snaps, bundleKeep, err := loadAllFromBundle(bundleHash, timestamp, txn)
			if err != nil { return err }
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
	if err != nil { return err }

	logs.Log.Debugf("Found %v value transactions. Applying to previous snapshot...", len(txs))
	for _, kv := range txs {
		var trimKey []byte
		err := db.DB.Update(func(txn *badger.Txn) error {
			// First: update snapshot balances
			address, err := db.GetBytes(db.AsKey(kv.key, db.KEY_ADDRESS_HASH), txn)
			if err != nil { return err }

			addressKey := db.GetByteKey(address, db.KEY_SNAPSHOT_BALANCE)

			_, err = db.IncrBy(addressKey, kv.value, false, txn)
			if err != nil { return err }

			// Update spents:
			if kv.value < 0 {
				err := db.Put(db.AsKey(addressKey, db.KEY_SNAPSHOT_SPENT), true, nil, txn)
				if err != nil { return err }
			}

			// Create trimming event:
			trimKey = db.AsKey(kv.key, db.KEY_EVENT_TRIM_PENDING)
			return db.Put(trimKey, true, nil, txn)
		})
		if err != nil { return err }
		edgeTransactions <- &trimKey
	}

	err = db.RemoveAll(db.KEY_PENDING_BUNDLE)
	if err != nil {
		return err
	}

	for _, key := range toKeepBundle {
		err := db.Put(key, true, nil, nil)
		if err != nil {
			return err
		}
	}

	for _, key := range snapshotted {
		err := db.Put(key, true, nil, nil)
		if err != nil {
			return err
		}
	}

	if checkDatabaseSnapshot() {
		logs.Log.Debug("Scheduling transaction trimming")
		trimData(int64(timestamp))
		err = db.DB.Update(func(txn *badger.Txn) error {
			err:= SetSnapshotTimestamp(timestamp, txn)
			if err != nil { return err }

			err = Unlock(txn)
			if err != nil { return err }
			db.RemoveAll(db.KEY_EDGE)
			path := config.GetString("snapshots.path")
			err = SaveSnapshot(path, timestamp)
			if err != nil { return err }
			logs.Log.Info("Snapshot finished and saved in", path)
			return nil
		})
		return err
	} else {
		return errors.New("failed database snapshot integrity check")
	}
}

func loadAllFromBundle (bundleHash []byte, timestamp int, txn *badger.Txn) ([]KeyValue, [][]byte, []byte, error) {
	var totalValue int64 = 0
	opts := badger.DefaultIteratorOptions
	opts.PrefetchValues = false
	it := txn.NewIterator(opts)
	defer it.Close()
	prefix := db.GetByteKey(bundleHash, db.KEY_BUNDLE)
	var txs []KeyValue
	var snapshotted [][]byte
	var nonZero = false

	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		k := it.Item().Key()
		key := make([]byte, 16)
		copy(key, k[16:])
		// Filter out unconfirmed reattachments:
		if !db.Has(db.AsKey(key, db.KEY_CONFIRMED), txn) {
			continue
		}

		txTimestamp, err := db.GetInt(db.AsKey(key, db.KEY_TIMESTAMP), txn)
		if err != nil {
			return nil, nil, nil, err
		}
		if txTimestamp > timestamp {
			snapshotted = append(snapshotted, db.AsKey(prefix, db.KEY_SNAPSHOTTED))
		}

		valueKey := db.AsKey(key, db.KEY_VALUE)
		value, err := db.GetInt64(valueKey, txn)
		if err == nil {
			if value != 0 {
				totalValue += value
				txs = append(txs, KeyValue{valueKey, value})
				nonZero = true
			}
		} else {
			logs.Log.Errorf("Error reading value for %v", valueKey)
			return nil, nil, nil, err
		}
	}
	// Probably debris from last snapshot. Has most probably to do with timestamps vs attachment timestamps
	// TODO: (OPT) investigate
	if totalValue != 0 || !nonZero {
		return nil, nil, db.AsKey(prefix, db.KEY_PENDING_BUNDLE), nil
		//return nil, nil, nil
		logs.Log.Errorf("A bundle is incomplete (non-zero sum). " +
			"The database is probably inconsistent or not in sync! %v", convert.BytesToTrytes(bundleHash)[:81])
		return nil, nil, nil, errors.New("A bundle is incomplete (non-zero sum). The database is probably inconsistent or not in sync!")
	}
	return txs, snapshotted, nil, nil
}

func ReplaceTimestamps () {
	var keys [][]byte
	err := db.DB.View(func(txn *badger.Txn) error {
		x := 0
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte{db.KEY_TIMESTAMP}
		logs.Log.Debug("Collecting all Keys...")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			k := item.Key()
			key := make([]byte, 16)
			copy(key, k)
			keys = append(keys, key)
			if x % 10000 == 0 {
				logs.Log.Debugf("PROGRESS: ", x)
			}
			x++
		}
		return nil
	})
	if err != nil {
		logs.Log.Error("WHOOPS1", err)
		return
	}
	logs.Log.Debug("Applying all Keys...")
	for x, key := range keys {
		err := db.DB.Update(func(txn *badger.Txn) error {
			txBytes, err := db.GetBytes(db.AsKey(key, db.KEY_BYTES), txn)
			if err != nil {
				return err
			}
			trits := convert.BytesToTrits(txBytes)[:8019]
			tx := transaction.TritsToFastTX(&trits, txBytes)
			err = db.Put(db.AsKey(key, db.KEY_TIMESTAMP), tx.Timestamp, nil, txn)
			if err != nil {
				return err
			}
			if x%10000 == 0 {
				logs.Log.Debugf("PROGRESS: ", x)
			}
			return nil
		})
		if err != nil {
			logs.Log.Error("WHOOPS2", err)
			break
		}
	}
	if err != nil {
		logs.Log.Error("WHOOPS3", err)
	}
}