package db

import (
	"sync"
)

// TODO: save trytes in a NoSQL DB
// TODO: reverse transactions to reflect approvees (memory only store)?

type DatabaseConfig struct {
	Path       string
	SavePeriod int
}

var conf *DatabaseConfig
var Locker = &sync.Mutex{}

func Snapshot () {
	// TODO: get old confirmed TXs
}

