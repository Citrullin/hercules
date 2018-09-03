package db

import (
	"fmt"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	"github.com/spf13/viper"

	"../logs"
)

func init() {
	RegisterImplementation("badger", NewBadger)
}

type Badger struct {
	db *badger.DB
	//locker        sync.Mutex
	cleanUpTicker *time.Ticker
}

func NewBadger(config *viper.Viper) (Interface, error) {
	path := config.GetString("database.path")
	light := config.GetBool("light")

	logs.Log.Infof("Loading database at %s", path)

	cleanUpInterval := 5 * time.Minute

	opts := badger.DefaultOptions
	opts.Dir = path
	opts.ValueDir = path
	opts.ValueLogLoadingMode = options.FileIO
	opts.TableLoadingMode = options.FileIO
	if light {
		// Source: https://github.com/dgraph-io/badger#memory-usage
		opts := badger.DefaultOptions
		opts.NumMemtables = 1
		opts.NumLevelZeroTables = 1
		opts.NumLevelZeroTablesStall = 2
		opts.NumCompactors = 1
		opts.MaxLevels = 5
		opts.LevelOneSize = 256 << 18
		opts.MaxTableSize = 64 << 18
		opts.ValueLogFileSize = 1 << 25
		opts.ValueLogMaxEntries = 250000
		cleanUpInterval = 2 * time.Minute
	}

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("open db [%s]: %v", path, err)
	}
	logs.Log.Info("Database loaded")

	b := &Badger{db: db, cleanUpTicker: time.NewTicker(cleanUpInterval)}
	b.cleanUp()
	go func() {
		for range b.cleanUpTicker.C {
			b.cleanUp()
		}
	}()
	return b, nil
}

func (b *Badger) Lock() {
	//b.locker.Lock()
}

func (b *Badger) Unlock() {
	//b.locker.Unlock()
}

func (b *Badger) PutBytes(key, value []byte, ttl *time.Duration) error {
	return b.Update(func(t Transaction) error {
		return t.PutBytes(key, value, ttl)
	})
}

func (b *Badger) GetBytes(key []byte) ([]byte, error) {
	tx := b.NewTransaction(false)
	defer tx.Discard()

	return tx.GetBytes(key)
}

func (b *Badger) GetBytesRaw(key []byte) ([]byte, error) {
	tx := b.NewTransaction(false)
	defer tx.Discard()

	return tx.GetBytesRaw(key)
}

func (b *Badger) HasKey(key []byte) bool {
	tx := b.NewTransaction(false)
	defer tx.Discard()

	return tx.HasKey(key)
}

func (b *Badger) HasKeysFromCategoryBefore(keyCategory byte, timestamp int) bool {
	tx := b.NewTransaction(false)
	defer tx.Discard()

	return tx.HasKeysFromCategoryBefore(keyCategory, timestamp)
}

func (b *Badger) Put(key []byte, value interface{}, ttl *time.Duration) error {
	return b.Update(func(t Transaction) error {
		return t.Put(key, value, ttl)
	})
}

func (b *Badger) Get(key []byte, value interface{}) error {
	tx := b.NewTransaction(false)
	defer tx.Discard()

	return tx.Get(key, value)
}

func (b *Badger) GetString(key []byte) (string, error) {
	tx := b.NewTransaction(false)
	defer tx.Discard()

	return tx.GetString(key)
}

func (b *Badger) GetInt(key []byte) (int, error) {
	tx := b.NewTransaction(false)
	defer tx.Discard()

	return tx.GetInt(key)
}

func (b *Badger) GetBool(key []byte) (bool, error) {
	tx := b.NewTransaction(false)
	defer tx.Discard()

	return tx.GetBool(key)
}

func (b *Badger) GetInt64(key []byte) (int64, error) {
	tx := b.NewTransaction(false)
	defer tx.Discard()

	return tx.GetInt64(key)
}

func (b *Badger) Remove(key []byte) error {
	return b.Update(func(t Transaction) error {
		return t.Remove(key)
	})
}

func (b *Badger) RemoveKeyCategory(keyCategory byte) error {
	return b.Update(func(t Transaction) error {
		return t.RemoveKeyCategory(keyCategory)
	})
}

func (b *Badger) RemoveKeysFromCategoryBefore(keyCategory byte, timestamp int64) (count int) {
	b.Update(func(t Transaction) error {
		count = t.RemoveKeysFromCategoryBefore(keyCategory, timestamp)
		return nil
	})
	return
}

func (b *Badger) RemovePrefix(prefix []byte) error {
	return b.Update(func(t Transaction) error {
		return t.RemovePrefix(prefix)
	})
}

func (b *Badger) CountKeyCategory(keyCategory byte) int {
	tx := b.NewTransaction(false)
	defer tx.Discard()

	return tx.CountKeyCategory(keyCategory)
}

func (b *Badger) CountPrefix(prefix []byte) int {
	tx := b.NewTransaction(false)
	defer tx.Discard()

	return tx.CountPrefix(prefix)
}

func (b *Badger) SumInt64FromCategory(keyCategory byte) int64 {
	tx := b.NewTransaction(false)
	defer tx.Discard()

	return tx.SumInt64FromCategory(keyCategory)
}

func (b *Badger) IncrementBy(key []byte, delta int64, deleteOnZero bool) (value int64, err error) {
	err = b.Update(func(t Transaction) error {
		value, err = t.IncrementBy(key, delta, deleteOnZero)
		return err
	})
	return
}

func (b *Badger) ForPrefix(prefix []byte, fetchValues bool, fn func([]byte, []byte) (bool, error)) error {
	tx := b.NewTransaction(false)
	defer tx.Discard()

	return tx.ForPrefix(prefix, fetchValues, fn)
}

func (b *Badger) NewTransaction(update bool) Transaction {
	return &BadgerTransaction{txn: b.db.NewTransaction(update)}
}

func (b *Badger) Update(fn func(Transaction) error) error {
	return b.db.Update(func(txn *badger.Txn) error {
		return fn(&BadgerTransaction{txn: txn})
	})
}

func (b *Badger) View(fn func(Transaction) error) error {
	return b.db.View(func(txn *badger.Txn) error {
		return fn(&BadgerTransaction{txn: txn})
	})
}

// Close locks the database for five seconds. Should be called before exiting.
// This is useful to allow running database processes to finished, but
// deny locking of new tasks.
func (b *Badger) Close() error {
	b.cleanUpTicker.Stop()
	//b.locker.Lock()
	time.Sleep(5 * time.Second)
	return b.db.Close()
}

func (b *Badger) cleanUp() {
	logs.Log.Debug("Cleanup database started")
	//b.locker.Lock()
	b.db.RunValueLogGC(0.5)
	//b.locker.Unlock()
	logs.Log.Debug("Cleanup database finished")
}
