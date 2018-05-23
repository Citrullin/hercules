package snapshot

import (
	"db"
	"github.com/dgraph-io/badger"
	"logs"
	"github.com/pkg/errors"
	"time"
)

func LoadSnapshot (path string) error {
	// TODO: write lock to the database. If this lock is present at load, the database is inconsistent!
	logs.Log.Debug("Loading snapshot", path)
	timestamp, err := checkSnapshotFile(path)
	if err != nil { return err }
	logs.Log.Debug("Timestamp:", timestamp)
	db.Locker.Lock()
	defer db.Locker.Unlock()

	// Give time for other processes to finalize
	time.Sleep(WAIT_SNAPSHOT_DURATION)

	logs.Log.Debug("Saving edge TXs")
	err = trimData(timestamp)
	logs.Log.Debug("Saved edge TXs", len(edgeTransactions))
	if err != nil { return err }

	// TODO: 4. Load snapshot data

	if checkDatabaseSnapshot() {
		return nil
	} else {
		return errors.New("failed database snapshot integrity check")
	}
}

func loadValueSnapshot(address []byte, value int64) error {
	addressKey := db.GetByteKey(address, db.KEY_SNAPSHOT_BALANCE)
	return db.DB.Update(func(txn *badger.Txn) error {
		err := db.PutBytes(db.AsKey(addressKey, db.KEY_ADDRESS_BYTES), address, nil, nil)
		if err != nil { return err }
		err = db.Put(addressKey, value, nil, nil)
		if err != nil { return err }
		err = db.Put(db.AsKey(addressKey, db.KEY_BALANCE), value, nil, nil)
		if err != nil { return err }
		return nil
	})
}

func loadSpentSnapshot(address []byte) error {
	addressKey := db.GetByteKey(address, db.KEY_SNAPSHOT_SPENT)
	return db.DB.Update(func(txn *badger.Txn) error {
		err := db.PutBytes(db.AsKey(addressKey, db.KEY_ADDRESS_BYTES), address, nil, nil)
		if err != nil { return err }
		err = db.Put(addressKey, true, nil, nil)
		if err != nil { return err }
		err = db.Put(db.AsKey(addressKey, db.KEY_SPENT), true, nil, nil)
		if err != nil { return err }
		return nil
	})
}
