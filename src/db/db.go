package db

import (
	"sync"
	"github.com/dgraph-io/badger"
	"log"
	"time"
)

const (
	dbCleanupInterval = time.Duration(5) * time.Minute
)

// TODO: (OPT) reverse transactions to reflect approvees (memory only store)?
// TODO: periodic snapshots
// TODO: (OPT) write tests

type DatabaseConfig struct {
	Path       string
	SavePeriod int
}

var DB *badger.DB
var Locker = &sync.Mutex{}

func Load(config *DatabaseConfig) {
	opts := badger.DefaultOptions
	opts.Dir = config.Path
	opts.ValueDir = config.Path
	db, err := badger.Open(opts)
	if err != nil {
		log.Fatal(err)
	}
	DB = db
	cleanupDB()
	go periodicDatabaseCleanup()
}

func End() {
	Locker.Lock()
	time.Sleep(time.Duration(5) * time.Second)
	DB.Close()
}

func periodicDatabaseCleanup () {
	for {
		time.Sleep(dbCleanupInterval)
		cleanupDB()
	}
}

func cleanupDB() {
	Locker.Lock()
	DB.PurgeOlderVersions()
	DB.RunValueLogGC(0.5)
	Locker.Unlock()
}

func Snapshot () {
	Locker.Lock()
	Locker.Unlock()
}

