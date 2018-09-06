package db

import (
	"fmt"
	"sync"
	"time"

	"../db/coding"
	"../logs"
	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	"github.com/spf13/viper"
)

var dbWaitGroup = &sync.WaitGroup{}

func init() {
	RegisterImplementation("badger", NewBadger)
}

type Badger struct {
	db            *badger.DB
	dbLock        *sync.Mutex
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

	b := &Badger{db: db, dbLock: &sync.Mutex{}, cleanUpTicker: time.NewTicker(cleanUpInterval)}
	b.cleanUp()
	go func() {
		for range b.cleanUpTicker.C {
			b.cleanUp()
		}
	}()
	return b, nil
}

func (b *Badger) Lock() {
	b.dbLock.Lock()
}

func (b *Badger) Unlock() {
	b.dbLock.Unlock()
}

func (b *Badger) PutBytes(key, value []byte) error {
	return b.Update(func(t Transaction) error {
		return coding.PutBytes(t, key, value)
	})
}

func (b *Badger) GetBytes(key []byte) ([]byte, error) {
	dbWaitGroup.Add(1)
	defer dbWaitGroup.Add(-1)

	tx := b.NewTransaction(false)
	defer tx.Discard()

	return coding.GetBytes(tx, key)
}

func (b *Badger) HasKey(key []byte) bool {
	dbWaitGroup.Add(1)
	defer dbWaitGroup.Add(-1)

	tx := b.NewTransaction(false)
	defer tx.Discard()

	return tx.HasKey(key)
}

func (b *Badger) Remove(key []byte) error {
	return b.Update(func(t Transaction) error {
		return coding.Remove(t, key)
	})
}

func (b *Badger) RemoveKeyCategory(keyCategory byte) error {
	return b.Update(func(t Transaction) error {
		return t.RemoveKeyCategory(keyCategory)
	})
}

func (b *Badger) RemovePrefix(prefix []byte) error {
	return b.Update(func(t Transaction) error {
		return t.RemovePrefix(prefix)
	})
}

func (b *Badger) CountKeyCategory(keyCategory byte) int {
	dbWaitGroup.Add(1)
	defer dbWaitGroup.Add(-1)

	tx := b.NewTransaction(false)
	defer tx.Discard()

	return tx.CountKeyCategory(keyCategory)
}

func (b *Badger) CountPrefix(prefix []byte) int {
	dbWaitGroup.Add(1)
	defer dbWaitGroup.Add(-1)

	tx := b.NewTransaction(false)
	defer tx.Discard()

	return tx.CountPrefix(prefix)
}

func (b *Badger) ForPrefix(prefix []byte, fetchValues bool, fn func([]byte, []byte) (bool, error)) error {
	dbWaitGroup.Add(1)
	defer dbWaitGroup.Add(-1)

	tx := b.NewTransaction(false)
	defer tx.Discard()

	return tx.ForPrefix(prefix, fetchValues, fn)
}

func (b *Badger) NewTransaction(update bool) Transaction {
	return &BadgerTransaction{txn: b.db.NewTransaction(update)}
}

func (b *Badger) Update(fn func(Transaction) error) error {
	dbWaitGroup.Add(1)
	defer dbWaitGroup.Add(-1)

	return b.db.Update(func(txn *badger.Txn) error {
		return fn(&BadgerTransaction{txn: txn})
	})
}

func (b *Badger) View(fn func(Transaction) error) error {
	dbWaitGroup.Add(1)
	defer dbWaitGroup.Add(-1)

	return b.db.View(func(txn *badger.Txn) error {
		return fn(&BadgerTransaction{txn: txn})
	})
}

// Close locks the database for five seconds. Should be called before exiting.
// This is useful to allow running database processes to finished, but
// deny locking of new tasks.
func (b *Badger) Close() error {
	b.cleanUpTicker.Stop()
	b.dbLock.Lock()
	time.Sleep(5 * time.Second)
	return b.db.Close()
}

func (b *Badger) cleanUp() {
	logs.Log.Debug("Cleanup database started")
	b.dbLock.Lock()
	b.db.RunValueLogGC(0.5)
	b.dbLock.Unlock()
	logs.Log.Debug("Cleanup database finished")
}

func (b *Badger) End() {
	dbWaitGroup.Wait()
}
