package db

import (
	"fmt"
	"sync"
	"time"

	"../config"
	"../db/coding"
	"../logs"
	"github.com/dgraph-io/badger"
)

func init() {
	RegisterImplementation("badger", startBadger)
}

type Badger struct {
	db            *badger.DB
	dbLock        *sync.Mutex
	cleanUpTicker *time.Ticker
	waitGroup     sync.WaitGroup
}

func startBadger() (Interface, error) {
	config.ConfigureBadger()

	logs.Log.Infof("Loading database at %s", config.BadgerOptions.Dir)
	db, err := badger.Open(config.BadgerOptions)
	if err != nil {
		return nil, fmt.Errorf("open db [%s]: %v", config.BadgerOptions.Dir, err)
	}
	logs.Log.Info("Database loaded")

	b := &Badger{db: db, dbLock: &sync.Mutex{}, cleanUpTicker: time.NewTicker(config.BadgerCleanUpInterval)}
	go b.cleanUp()
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
		return t.PutBytes(key, value)
	})
}

func (b *Badger) GetBytes(key []byte) ([]byte, error) {
	b.waitGroup.Add(1)
	defer b.waitGroup.Add(-1)

	tx := b.NewTransaction(false)
	defer tx.Discard()

	return tx.GetBytes(key)
}

func (b *Badger) HasKey(key []byte) bool {
	b.waitGroup.Add(1)
	defer b.waitGroup.Add(-1)

	tx := b.NewTransaction(false)
	defer tx.Discard()

	return tx.HasKey(key)
}

func (b *Badger) Remove(key []byte) error {
	return b.Update(func(t Transaction) error {
		return coding.Remove(t, key)
	})
}

func (b *Badger) RemovePrefix(prefix []byte) error {
	return b.Update(func(t Transaction) error {
		return t.RemovePrefix(prefix)
	})
}

func (b *Badger) CountPrefix(prefix []byte) int {
	b.waitGroup.Add(1)
	defer b.waitGroup.Add(-1)

	tx := b.NewTransaction(false)
	defer tx.Discard()

	return tx.CountPrefix(prefix)
}

func (b *Badger) ForPrefix(prefix []byte, fetchValues bool, fn func([]byte, []byte) (bool, error)) error {
	b.waitGroup.Add(1)
	defer b.waitGroup.Add(-1)

	tx := b.NewTransaction(false)
	defer tx.Discard()

	return tx.ForPrefix(prefix, fetchValues, fn)
}

func (b *Badger) NewTransaction(update bool) Transaction {
	return &BadgerTransaction{txn: b.db.NewTransaction(update)}
}

func (b *Badger) Update(fn func(Transaction) error) error {
	b.waitGroup.Add(1)
	defer b.waitGroup.Add(-1)

	return b.db.Update(func(txn *badger.Txn) error {
		return fn(&BadgerTransaction{txn: txn})
	})
}

func (b *Badger) View(fn func(Transaction) error) error {
	b.waitGroup.Add(1)
	defer b.waitGroup.Add(-1)

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
	executeCleanUp(b)

	for range b.cleanUpTicker.C {
		executeCleanUp(b)
	}
}

func executeCleanUp(b *Badger) {
	logs.Log.Debug("Cleanup database started")
	b.dbLock.Lock()
	b.db.RunValueLogGC(0.5)
	b.dbLock.Unlock()
	logs.Log.Debug("Cleanup database finished")
}

func (b *Badger) End() {
	b.waitGroup.Wait()
}
