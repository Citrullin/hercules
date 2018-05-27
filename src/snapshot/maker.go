package snapshot

import (
	"db"
	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
	"logs"
	"bytes"
	"encoding/gob"
	"convert"
	"transaction"
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

	var bundles [][]byte
	var txs []KeyValue

	contains := func(key []byte) bool {
		if bundles == nil { return false }
		for _, k := range bundles {
			if bytes.Equal(k, key) { return true }
		}
		return false
	}

	err := db.DB.View(func(txn *badger.Txn) error {
		if !IsEqualOrNewerThanSnapshot(int(timestamp), nil) {
			logs.Log.Infof("The given snapshot (%v) timestamp is older than the current one. Skipping", timestamp)
			return errors.New("given snapshot is older than current one")
		}
		if !IsSynchronized() {
			logs.Log.Warning("Tangle not fully synchronized - cannot create snapshot!")
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
		logs.Log.Debug("Collecting all bundles before the snapshot horizon...")
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
		logs.Log.Debugf("Found %v bundles. Colleting corresponsing transactions...", len(bundles))
		for _, bundleHash := range bundles {
			bundleTxs, err := loadAllFromBundle(bundleHash, txn)
			if err != nil { return err }
			txs = append(txs, bundleTxs...)
		}
		return nil
	})
	if err != nil { return err }

	// TODO: change to debug
	logs.Log.Fatalf("Found %v transactions. Applying to snapshot...", len(txs))
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

	if checkDatabaseSnapshot() {
		logs.Log.Debug("Scheduling transaction trimming")
		trimData(int64(timestamp))
		return db.DB.Update(func(txn *badger.Txn) error {
			err:= SetSnapshotTimestamp(timestamp, txn)
			if err != nil { return err }

			err = Unlock(txn)
			if err != nil { return err }
			path := config.GetString("snapshots.path")
			err = SaveSnapshot(path)
			if err != nil { return err }
			logs.Log.Info("Snapshot finished and saved in", path)
			return nil
		})
	} else {
		return errors.New("failed database snapshot integrity check")
	}
}

func loadAllFromBundle (bundleHash []byte, txn *badger.Txn) ([]KeyValue, error) {
	var totalValue int64 = 0
	opts := badger.DefaultIteratorOptions
	opts.PrefetchValues = false
	it := txn.NewIterator(opts)
	defer it.Close()
	prefix := db.GetByteKey(bundleHash, db.KEY_BUNDLE)
	var txs []KeyValue
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		k := it.Item().Key()
		key := make([]byte, 16)
		copy(key, k[16:])
		valueKey := db.AsKey(key, db.KEY_VALUE)
		value, err := db.GetInt64(valueKey, txn)
		if err == nil {
			if value != 0 && db.Has(db.AsKey(key, db.KEY_CONFIRMED), txn){
				totalValue += value
				txs = append(txs, KeyValue{valueKey, value})
			}
		} else {
			logs.Log.Errorf("Error reading value for %v", valueKey)
			return nil, err
		}
	}
	if totalValue != 0 {
		logs.Log.Warningf(convert.BytesToTrytes(bundleHash), totalValue)
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := db.GetByteKey(bundleHash, db.KEY_BUNDLE)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var index int64
			k := it.Item().Key()
			v, _ := it.Item().Value()
			buf := bytes.NewBuffer(v)
			dec := gob.NewDecoder(buf)
			err := dec.Decode(&index)

			key := make([]byte, 16)
			copy(key, k[16:])
			valueKey := db.AsKey(key, db.KEY_VALUE)
			value, err := db.GetInt64(valueKey, txn)
			if err == nil {
				if value != 0 && db.Has(db.AsKey(key, db.KEY_CONFIRMED), txn){
					hash,_ := db.GetBytes(db.AsKey(key, db.KEY_HASH), txn)
					logs.Log.Warningf("   -> %v: %v -> %v", index, convert.BytesToTrytes(hash)[:81], value)
				}
			}
		}
		return nil, nil
	}
	return txs, nil
}