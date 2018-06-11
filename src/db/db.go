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
	dbCleanupIntervalLight = time.Duration(1) * time.Minute
)

// TODO: (OPT) write tests for the database?

var DB *badger.DB
var config *viper.Viper
var Locker = &sync.Mutex{}
var LatestTransactionTimestamp = 0

/*
Loads the database and configures according the the config options.
 */
func Load(cfg *viper.Viper) {
	logs.Log.Info("Loading database")

	config = cfg

	opts := badger.DefaultOptions
	opts.Dir = config.GetString("database.path")
	opts.ValueDir = opts.Dir

	// Source: https://github.com/dgraph-io/badger#memory-usage
	if config.GetBool("light") {
		opts.ValueLogLoadingMode = options.FileIO
		opts.TableLoadingMode = options.FileIO
		opts.NumMemtables = 1
		opts.NumLevelZeroTables = 1
		opts.NumLevelZeroTablesStall = 2
		opts.NumCompactors = 1
		opts.MaxLevels = 7
		opts.LevelOneSize = 256 << 18
		opts.MaxTableSize = 64 << 18
		//opts.ValueLogFileSize = 1 << 31
		//opts.ValueLogMaxEntries = 10000000
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

/*
Locks the database for five seconds. Should be called before exiting.
This is useful to allow running database processes to finished, but
deny locking of new tasks.
 */
func End() {
	Locker.Lock()
	time.Sleep(time.Duration(5) * time.Second)
	DB.Close()
}

/*
Runner for database garbage collection.
 */
func periodicDatabaseCleanup () {
	var duration = dbCleanupInterval
	if config.GetBool("light") {
		duration = dbCleanupIntervalLight
	}
	for {
		time.Sleep(duration)
		cleanupDB()
	}
}

/*
Garbage-collects debris from the memory.
 */
func cleanupDB() {
	logs.Log.Debug("Cleanup database started")
	Locker.Lock()
	DB.RunValueLogGC(0.5)
	Locker.Unlock()
	logs.Log.Debug("Cleanup database finished")
}

