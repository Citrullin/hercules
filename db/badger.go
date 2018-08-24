package db

import (
	"fmt"
	"sync"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"

	"../logs"
)

type Badger struct {
	db     *badger.DB
	locker sync.Mutex
}

func NewBadger(path string, light bool) (*Badger, error) {
	logs.Log.Info("Loading database")

	opts := badger.DefaultOptions
	opts.Dir = path
	opts.ValueDir = path

	opts.ValueLogLoadingMode = options.FileIO
	opts.TableLoadingMode = options.FileIO
	// Source: https://github.com/dgraph-io/badger#memory-usage
	if light {
		opts.NumMemtables = 1
		opts.NumLevelZeroTables = 1
		opts.NumLevelZeroTablesStall = 2
		opts.NumCompactors = 1
		opts.MaxLevels = 5
		opts.LevelOneSize = 256 << 18
		opts.MaxTableSize = 64 << 18
		opts.ValueLogFileSize = 1 << 25
		opts.ValueLogMaxEntries = 250000
	}

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("open db [%s]: %v", path, err)
	}
	logs.Log.Info("Database loaded")

	b := &Badger{db: db}
	b.cleanUp()
	return b, nil
}

// Close locks the database for five seconds. Should be called before exiting.
// This is useful to allow running database processes to finished, but
// deny locking of new tasks.
func (b *Badger) Close() error {
	b.locker.Lock()
	time.Sleep(5 * time.Second)
	return b.db.Close()
}

func (b *Badger) NewTransaction(update bool) *BadgerTransaction {
	return &BadgerTransaction{txn: b.db.NewTransaction(update)}
}

func (b *Badger) PutBytes(key, value []byte, ttl *time.Duration) error {
	return b.update(func(bt *BadgerTransaction) error {
		return bt.PutBytes(key, value, ttl)
	})
}

func (b *Badger) GetBytes(key []byte) ([]byte, error) {
	bt := b.NewTransaction(false)
	defer bt.Discard()

	return bt.GetBytes(key)
}

func (b *Badger) update(fn func(*BadgerTransaction) error) error {
	return b.db.Update(func(txn *badger.Txn) error {
		return fn(&BadgerTransaction{txn: txn})
	})
}

func (b *Badger) cleanUp() {
	logs.Log.Debug("Cleanup database started")
	b.locker.Lock()
	b.db.RunValueLogGC(0.5)
	b.locker.Unlock()
	logs.Log.Debug("Cleanup database finished")
}
