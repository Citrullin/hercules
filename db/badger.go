package db

import (
	"fmt"
	"sync"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	"github.com/spf13/viper"

	"../logs"
)

func init() {
	implementations["badger"] = NewBadger
}

type Badger struct {
	db     *badger.DB
	locker sync.Mutex
}

func NewBadger(config *viper.Viper) (Interface, error) {
	path := config.GetString("database.path")
	light := config.GetBool("light")

	logs.Log.Info("Loading database at %s", path)

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

func (b *Badger) GetBytesRaw(key []byte) ([]byte, error) {
	bt := b.NewTransaction(false)
	defer bt.Discard()

	return bt.GetBytesRaw(key)
}

func (b *Badger) Has(key []byte) bool {
	bt := b.NewTransaction(false)
	defer bt.Discard()

	return bt.Has(key)
}

func (b *Badger) Put(key []byte, value interface{}, ttl *time.Duration) error {
	return b.update(func(bt *BadgerTransaction) error {
		return bt.Put(key, value, ttl)
	})
}

func (b *Badger) Get(key []byte, value interface{}) error {
	bt := b.NewTransaction(false)
	defer bt.Discard()

	return bt.Get(key, value)
}

func (b *Badger) GetString(key []byte) (string, error) {
	bt := b.NewTransaction(false)
	defer bt.Discard()

	return bt.GetString(key)
}

func (b *Badger) GetInt(key []byte) (int, error) {
	bt := b.NewTransaction(false)
	defer bt.Discard()

	return bt.GetInt(key)
}

func (b *Badger) GetBool(key []byte) (bool, error) {
	bt := b.NewTransaction(false)
	defer bt.Discard()

	return bt.GetBool(key)
}

func (b *Badger) GetInt64(key []byte) (int64, error) {
	bt := b.NewTransaction(false)
	defer bt.Discard()

	return bt.GetInt64(key)
}

func (b *Badger) Remove(key []byte) error {
	return b.update(func(bt *BadgerTransaction) error {
		return bt.Remove(key)
	})
}

func (b *Badger) RemoveKeyCategory(keyCategory byte) error {
	return b.update(func(bt *BadgerTransaction) error {
		return bt.RemoveKeyCategory(keyCategory)
	})
}

func (b *Badger) RemoveKeysFromCategoryBefore(keyCategory byte, timestamp int64) (count int) {
	b.update(func(bt *BadgerTransaction) error {
		count = bt.RemoveKeysFromCategoryBefore(keyCategory, timestamp)
		return nil
	})
	return
}

func (b *Badger) RemovePrefix(prefix []byte) error {
	return b.update(func(bt *BadgerTransaction) error {
		return bt.RemovePrefix(prefix)
	})
}

func (b *Badger) CountKeyCategory(keyCategory byte) int {
	bt := b.NewTransaction(false)
	defer bt.Discard()

	return bt.CountKeyCategory(keyCategory)
}

func (b *Badger) CountPrefix(prefix []byte) int {
	bt := b.NewTransaction(false)
	defer bt.Discard()

	return bt.CountPrefix(prefix)
}

func (b *Badger) IncrementBy(key []byte, delta int64, deleteOnZero bool) (value int64, err error) {
	err = b.update(func(bt *BadgerTransaction) error {
		value, err = bt.IncrementBy(key, delta, deleteOnZero)
		return err
	})
	return
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
