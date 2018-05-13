package db

import (
	"sync"
	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	"log"
	"time"
)

const (
	dbCleanupInterval = time.Duration(5) * time.Minute
)

// TODO: (OPT) periodic snapshots
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
	// TODO: only for small devices? Compare performance, add as configurable
	// Also, tweak these: https://github.com/dgraph-io/badger#memory-usage
	if true {
		opts.ValueLogLoadingMode = options.FileIO
		opts.NumLevelZeroTables = 2
		opts.NumLevelZeroTablesStall = 4
		opts.NumMemtables = 2
		opts.NumCompactors = 1
		opts.TableLoadingMode = options.FileIO
		opts.MaxTableSize = 64 << 15
		opts.ValueLogFileSize = 1 << 25
	}
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

