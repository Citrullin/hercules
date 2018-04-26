package tangle

import (
	"github.com/garyburd/redigo/redis"
	"fmt"
	"time"
	"runtime"
	"sync"
	"db"
	"convert"
	"github.com/dgraph-io/badger"
)

const (
	maxQueueSize = 1000000
	flushInterval = time.Duration(10) * time.Second
	pingInterval = time.Duration(500) * time.Millisecond
	cooAddress = "KPWCHICGJZXKE9GSUDXZYUAPLHAKAHYHDXNPHENTERYMMBQOPSQIDENXKLKCEYCPVTZQLEEJVYJZV9BWU"
)

type Message struct {
	Trytes *[]byte
	Requested *[]byte
	Addr string
}

type messageQueue chan *Message

type Tangle struct {
	Incoming messageQueue
	Outgoing messageQueue
}

var nbWorkers = runtime.NumCPU()
var Pool *redis.Pool
var tangle *Tangle

type Requests struct {
	Data map[string]time.Time
	sync.RWMutex
}

type TXS struct {
	Data map[string]*TX
	sync.RWMutex
}
var requests Requests
var txs TXS
var received = 0
var confirmed = 0

func Start () *Tangle {
	txs.Data = make(map[string]*TX)
	requests.Data = make(map[string]time.Time)
	tangle = &Tangle{
		Incoming: make(messageQueue, maxQueueSize),
		Outgoing: make(messageQueue, maxQueueSize)}
	go requestTip()
	for i := 0; i < nbWorkers; i++ {
		go listenToIncoming()
	}
	go report()
	return tangle
}

func listenToIncoming () {
	for msg := range tangle.Incoming {
		_ = db.DB.Update(func(txn *badger.Txn) error {
			trytes := convert.BytesToTrytes(*msg.Trytes)[:2673]
			tx := TrytesToObject(trytes)
			db.Remove(db.GetHashKey(tx.Hash, db.KEY_REQUESTS), txn)
			milestone := isMilestone(tx)

			if !db.Has(db.GetHashKey(tx.Hash, db.KEY_HASH), txn) {
				trunkBytes := convert.TrytesToBytes(tx.TrunkTransaction)
				branchBytes := convert.TrytesToBytes(tx.BranchTransaction)
				trunk := db.GetByteKey(trunkBytes, db.KEY_TRANSACTION)
				branch := db.GetByteKey(branchBytes, db.KEY_TRANSACTION)
				both := make([]byte, 32)
				copy(both, trunk)
				copy(both[16:], branch)

				db.Put(db.GetHashKey(tx.Hash, db.KEY_HASH), convert.TrytesToBytes(tx.Hash), txn)
				db.Put(db.GetHashKey(tx.Hash, db.KEY_TIMESTAMP), tx.Timestamp, txn)
				db.Put(db.GetHashKey(tx.Hash, db.KEY_TRANSACTION), *msg.Trytes, txn)
				//db.Put(db.GetHashKey(tx.Hash, db.KEY_RELATION), both, txn)
				if milestone {
					db.Put(db.GetHashKey(tx.Hash, db.KEY_MILESTONE), both, txn)
				}

				if !db.Has(db.GetHashKey(tx.TrunkTransaction, db.KEY_HASH), txn) && !db.Has(db.GetHashKey(tx.TrunkTransaction, db.KEY_REQUESTS), txn) {
					db.Put(db.GetHashKey(tx.TrunkTransaction, db.KEY_REQUESTS), time.Now().Unix(), txn)
					// TODO: try a milestone first, then a recent TX, then empty TX
					tangle.Outgoing <- &Message{msg.Trytes, &trunkBytes, msg.Addr}
				}
				if !db.Has(db.GetHashKey(tx.BranchTransaction, db.KEY_HASH), txn) && !db.Has(db.GetHashKey(tx.BranchTransaction, db.KEY_REQUESTS), txn) {
					db.Put(db.GetHashKey(tx.TrunkTransaction, db.KEY_REQUESTS), time.Now().Unix(), txn)
					// TODO: try a milestone first, then a recent TX, then empty TX
					tangle.Outgoing <- &Message{msg.Trytes, &branchBytes, msg.Addr}
				}
				received++
			}

			if milestone || db.Has(db.GetHashKey(tx.Hash, db.KEY_UNKNOWN), txn) {
				confirm(tx, txn)
			}
			return nil
		})
	}
}

func confirm (tx *TX, txn *badger.Txn) {
	db.Remove(db.GetHashKey(tx.Hash, db.KEY_UNKNOWN), txn)
	db.Put(db.GetHashKey(tx.Hash, db.KEY_CONFIRMED), tx.Timestamp, txn)
	confirmed++
	confirmChild(tx.TrunkTransaction, txn)
	confirmChild(tx.BranchTransaction, txn)
}

func confirmChild (hash string, txn *badger.Txn) {
	if db.Has(db.GetHashKey(hash, db.KEY_CONFIRMED), txn) { return }
	if db.Has(db.GetHashKey(hash, db.KEY_TRANSACTION), txn) {
		var bytes []byte
		err := db.Get(db.GetHashKey(hash, db.KEY_TRANSACTION), &bytes, txn)
		if err != nil {
			tx := TrytesToObject(convert.BytesToTrytes(bytes[:1604])[:2673])
			if tx != nil {
				confirm(tx, txn)
			}
		} else {
			// TODO: remove all and re-request the transaction?
		}
	} else {
		db.Put(db.GetHashKey(hash, db.KEY_UNKNOWN), time.Now().Unix(), txn)
		// TODO: not random, return milestone or so
		tx := make([]byte, 1604)
		r := convert.TrytesToBytes(hash)
		tangle.Outgoing <- &Message{&tx, &r, ""}
	}

}

func isMilestone(tx *TX) bool {
	return tx.Address == cooAddress
}

func requestTip () {
	ticker := time.NewTicker(pingInterval)
	for range ticker.C {
		tx := make([]byte, 1604)
		r := make([]byte, 46)
		tangle.Outgoing <- &Message{&tx, &r, ""}
	}
}

func report () {
	flushTicker := time.NewTicker(flushInterval)
	for range flushTicker.C {
		fmt.Printf("TXs: %v/%v, Queue: %v PEnding: %v\n", confirmed, received, len(tangle.Incoming), db.Count(db.KEY_REQUESTS))
	}
}

// TODO: periodically re-request old TXs
// TODO: return knows TXs
