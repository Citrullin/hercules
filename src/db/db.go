package db

import (
	"sync"
	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	"time"
	"logs"
)

const (
	dbCleanupInterval = time.Duration(5) * time.Minute
)

// TODO: load snapshot
// TODO: import snapshot data from IRI
// TODO: (OPT) periodic snapshots
// TODO: (OPT) write tests

type DatabaseConfig struct {
	Path       string
	SavePeriod int
}

var DB *badger.DB
var Locker = &sync.Mutex{}

func Load(config *DatabaseConfig) {
	logs.Log.Info("Loading database")
	opts := badger.DefaultOptions
	opts.Dir = config.Path
	opts.ValueDir = config.Path
	// TODO: only for small devices? Compare performance, add as configurable
	// Source: https://github.com/dgraph-io/badger#memory-usage
	if true {
		opts.ValueLogLoadingMode = options.FileIO
		opts.NumLevelZeroTables = 2
		opts.NumLevelZeroTablesStall = 3
		opts.NumMemtables = 2
		opts.NumCompactors = 2
		opts.TableLoadingMode = options.FileIO
		//opts.MaxTableSize = 64 << 12
		//opts.ValueLogFileSize = 1 << 24
	}
	db, err := badger.Open(opts)
	if err != nil {
		logs.Log.Fatal(err)
	}
	DB = db
	cleanupDB()
	go periodicDatabaseCleanup()
	logs.Log.Info("Database loaded")
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
	logs.Log.Info("Cleanup database started")
	Locker.Lock()
	DB.RunValueLogGC(0.5)
	Locker.Unlock()
	logs.Log.Info("Cleanup database finished")
}

func Snapshot () {
	Locker.Lock()
	Locker.Unlock()
}

