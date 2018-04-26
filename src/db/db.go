package db

import (
	"sync"
	"github.com/dgraph-io/badger"
	"log"
)

// TODO: reverse transactions to reflect approvees (memory only store)?
// TODO: periodic snapshots
// TODO: write tests

type DatabaseConfig struct {
	Path       string
	SavePeriod int
}

var conf *DatabaseConfig
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
	db.PurgeOlderVersions()
	db.RunValueLogGC(0.5)
}

func End() {
	Locker.Lock()
	DB.Close()
}

func Snapshot () {
	Locker.Lock()
	Locker.Unlock()
}

