package db

import (
	"time"
	"sync"
	"os"
	"path"
)

// TODO: save trytes in a NoSQL DB
// TODO: reverse transactions to reflect approvees (memory only store)?

type DatabaseConfig struct {
	Path       string
	SavePeriod int
}

var Transactions *TransactionStore
var Confirmations *LockableTimedHashStore
var Milestones *LockableTimedHashStore
var UnknownConfirmations *LockableTimedHashStore
var Requests *LockableTimedHashStore
var conf *DatabaseConfig
var Locker = &sync.Mutex{}

func Load (config *DatabaseConfig) {
	conf = config
	objectsPath := path.Join(config.Path, "objects")
	os.MkdirAll(conf.Path, os.ModePerm)
	os.MkdirAll(objectsPath, os.ModePerm)

	Transactions, _ = LoadTransactionStore(
		path.Join(config.Path, "transactions.dat"), objectsPath)
	Confirmations, _ = LoadTimedStore(path.Join(config.Path, "confirmations.dat"))
	UnknownConfirmations, _ = LoadTimedStore(path.Join(config.Path, "unknowns.dat"))
	Requests, _ = LoadTimedStore(path.Join(config.Path, "requests.dat"))
	Milestones, _ = LoadTimedStore(path.Join(config.Path, "milestones.dat"))
	saveWorker()
}

func End () {
	Save()
}

func saveWorker () {
	saveTicker := time.NewTicker(time.Duration(conf.SavePeriod) * time.Second)
	go func() {
		for range saveTicker.C {
			Save()
		}
	}()
}

// TODO: move original, save, delete backup
func Save () {
	Locker.Lock()
	Transactions.save(path.Join(conf.Path, "transactions.dat"))
	Confirmations.save(path.Join(conf.Path, "confirmations.dat"))
	UnknownConfirmations.save(path.Join(conf.Path, "unknowns.dat"))
	Requests.save(path.Join(conf.Path, "requests.dat"))
	Milestones.save(path.Join(conf.Path, "milestones.dat"))
	Locker.Unlock()
}

func Delete (hash string) {
	Transactions.Delete(hash)
	Confirmations.Delete(hash)
	Milestones.Delete(hash)
}

func Snapshot () {
	// TODO: get old confirmed TXs
}

