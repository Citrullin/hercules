package db

import (
	"sync"
	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	"time"
	"logs"
	"github.com/spf13/viper"
)

const (
	dbCleanupInterval = time.Duration(5) * time.Minute
)

// TODO: load snapshot
// TODO: import snapshot data from IRI
// TODO: (OPT) periodic snapshots
// TODO: (OPT) write tests

var DB *badger.DB
var config *viper.Viper
var Locker = &sync.Mutex{}

func Load(cfg *viper.Viper) {
	logs.Log.Info("Loading database")

	config = cfg

	opts := badger.DefaultOptions
	opts.Dir = config.GetString("database.path")
	opts.ValueDir = opts.Dir
	// TODO: only for small devices? Compare performance, add as configurable
	// Source: https://github.com/dgraph-io/badger#memory-usage
	if true {
		opts.ValueLogLoadingMode = options.FileIO
		opts.TableLoadingMode = options.FileIO
		opts.NumMemtables = 1
		opts.NumLevelZeroTables = 1
		opts.NumLevelZeroTablesStall = 2
		opts.NumCompactors = 1
		//opts.MaxTableSize = 64 << 10
		opts.ValueLogFileSize = 1 << 27
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

