package tangle

import (
	"github.com/garyburd/redigo/redis"
	"fmt"
	"time"
	"runtime"
	"strings"
	"sync"
	"db"
)

const (
	maxQueueSize = 1000000
	flushInterval = time.Duration(10) * time.Second
	pingInterval = time.Duration(500) * time.Millisecond
	cooAddress = "KPWCHICGJZXKE9GSUDXZYUAPLHAKAHYHDXNPHENTERYMMBQOPSQIDENXKLKCEYCPVTZQLEEJVYJZV9BWU"
)

type Message struct {
	Trytes string
	Requested string
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
		db.Locker.Lock()
		db.Locker.Unlock()
		tx := TrytesToObject(msg.Trytes)
		db.Requests.Delete(tx.Hash)

		if !db.Transactions.Has(tx.Hash) {
			db.Transactions.Save(tx.Hash, &db.DatabaseTransaction{tx.TrunkTransaction, tx.BranchTransaction}, &msg.Trytes)
			if !db.Transactions.Has(tx.TrunkTransaction) && !db.Requests.Has(tx.TrunkTransaction) {
				db.Requests.SaveNow(tx.TrunkTransaction)
				// TODO: try a milestone first, then a recent TX, then empty TX
				tangle.Outgoing <- &Message{msg.Trytes, tx.TrunkTransaction, msg.Addr}
			}
			if !db.Transactions.Has(tx.BranchTransaction) && !db.Requests.Has(tx.BranchTransaction) {
				db.Requests.SaveNow(tx.BranchTransaction)
				// TODO: try a milestone first, then a recent TX, then empty TX
				tangle.Outgoing <- &Message{msg.Trytes, tx.BranchTransaction, msg.Addr}
			}
		}

		if isMaster(tx) || db.UnknownConfirmations.Has(tx.Hash) {
			go confirm(tx)
		}
	}
}

func confirm (tx *TX) {
	db.UnknownConfirmations.Delete(tx.Hash)
	db.Confirmations.Save(tx.Hash, time.Unix(int64(tx.Timestamp), 0))
	confirmChild(tx.TrunkTransaction)
	confirmChild(tx.BranchTransaction)
}

func confirmChild (hash string) {
	if db.Confirmations.Has(hash) { return }
	if db.Transactions.Has(hash) {
		trytes, err := db.Transactions.GetTrytes(hash)
		if err != nil {
			tx := TrytesToObject(trytes)
			if tx != nil {
				confirm(tx)
			}
		} else {
			// TODO: remove and re-request the transaction
		}
	} else {
		db.UnknownConfirmations.SaveNow(hash)
		// TODO: not random, return milestone or so
		tangle.Outgoing <- &Message{strings.Repeat("9", 2673), hash, ""}
	}

}

func isMaster(tx *TX) bool {
	return tx.Address == cooAddress
}

func getRandomMessage () *Message {
	return &Message{strings.Repeat("9", 2673), strings.Repeat("9", 81), ""}
}

func requestTip () {
	ticker := time.NewTicker(pingInterval)
	for range ticker.C {
		tangle.Outgoing <- getRandomMessage()
	}
}

func report () {
	flushTicker := time.NewTicker(flushInterval)
	for range flushTicker.C {
		fmt.Printf("TXs: %v/%v, Queue: %v, Requests: %v \n", db.Confirmations.Count(), db.Transactions.Count(), len(tangle.Incoming), db.Requests.Count())
	}
}
