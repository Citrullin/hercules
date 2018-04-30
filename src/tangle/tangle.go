package tangle

import (
	"fmt"
	"time"
	"runtime"
	"db"
	"convert"
	"github.com/dgraph-io/badger"
	"strings"
	"bytes"
	"utils"
	"crypt"
	"server"
)

const (
	MWM = 14
	maxQueueSize = 100000
	flushInterval = time.Duration(10) * time.Second
	pingInterval = time.Duration(100) * time.Millisecond
	tipPingInterval = time.Duration(100) * time.Millisecond
	cooAddress = "KPWCHICGJZXKE9GSUDXZYUAPLHAKAHYHDXNPHENTERYMMBQOPSQIDENXKLKCEYCPVTZQLEEJVYJZV9BWU"
)

type Message struct {
	Bytes     *[]byte
	Requested *[]byte
	Addr      string
}

type Request struct {
	Requested []byte
	Addr string
	Tip bool
}

type RequestQueue chan *Request
type MessageQueue chan *Message
type TXQueue chan *FastTX

var srv *server.Server
var nbWorkers = runtime.NumCPU()
var requestReplyQueue RequestQueue
var outgoingQueue MessageQueue
var incomingQueue MessageQueue

var latestMilestoneKey = []byte("MilestoneLatest")
var tipBytes = convert.TrytesToBytes(strings.Repeat("9", 2673))[:1604]
var tipTrits = convert.BytesToTrits(tipBytes)[:8019]
var tipFastTX = TritsToFastTX(&tipTrits)
var coo = convert.TrytesToBytes(cooAddress)[:49]
var fingerprintTTL = time.Duration(10) * time.Minute
var reRequestTTL = time.Duration(5) * time.Second

var incoming = 0
var incomingProcessed = 0
var outgoing = 0
var outgoingProcessed = 0
var saved = 0
var discarded = 0

func Start (s *server.Server) {
	srv = s
	incomingQueue = make(MessageQueue, maxQueueSize)
	outgoingQueue = make(MessageQueue, maxQueueSize)
	requestReplyQueue = make(RequestQueue, maxQueueSize)

	go periodicRequest()
	go periodicTipRequest()

	for i := 0; i < nbWorkers / 2; i++ {
		go incomingRunner()
		go listenToIncoming()
		go requestReplyRunner()
		go responseRunner()
	}
	go report()
}

func incomingRunner () {
	for raw := range srv.Incoming {
		data := raw.Msg[:1604]
		req := raw.Msg[1604:1650]
		msg := &Message{&data,&req, raw.Addr}

		incoming++

		db.Locker.Lock()
		db.Locker.Unlock()

		_ = db.DB.Update(func(txn *badger.Txn) error {
			var fingerprint []byte
			var has = bytes.Equal(*msg.Bytes, tipBytes)

			if !has {
				fingerprint = db.GetByteKey(*msg.Bytes, db.KEY_FINGERPRINT)
				if !db.Has(fingerprint, txn) {
					db.Put(fingerprint, true, &fingerprintTTL, txn)
					incomingQueue <- msg
					incomingProcessed++
				}
			}
			return nil
		})
	}
}

func listenToIncoming () {
	for msg := range incomingQueue {
		trits := convert.BytesToTrits(*msg.Bytes)[:8019]
		tx := TritsToFastTX(&trits)
		if !crypt.IsValidPoW(tx.Hash, MWM) {
			server.NeighborTrackingQueue <- &server.NeighborTrackingMessage{Addr: msg.Addr, Invalid: 1}
			continue
		}

		db.Locker.Lock()
		db.Locker.Unlock()

		_ = db.DB.Update(func(txn *badger.Txn) error {
			db.Remove(db.GetByteKey(tx.Hash, db.KEY_REQUESTS), txn)
			db.Remove(db.GetByteKey(tx.Hash, db.KEY_REQUESTS_HASH), txn)

			// TODO: check if the TX is recent (than snapshot). Otherwise drop.

			if !db.Has(db.GetByteKey(tx.Hash, db.KEY_HASH), txn) {
				db.Put(db.GetByteKey(tx.Hash, db.KEY_HASH), tx.Hash, nil, txn)
				db.Put(db.GetByteKey(tx.Hash, db.KEY_TIMESTAMP), tx.Timestamp, nil, txn)
				db.Put(db.GetByteKey(tx.Hash, db.KEY_TRANSACTION), (*msg.Bytes)[:1604], nil, txn)
				milestone := isMilestone(tx)
				if milestone {
					milestoneKey := db.GetByteKey(tx.Hash, db.KEY_MILESTONE)

					// Update the latest milestone saved
					_, timestamp, err := db.GetLatestKey(db.KEY_MILESTONE, txn)
					if err != nil || timestamp < tx.Timestamp {
						db.Put(latestMilestoneKey, milestoneKey, nil, txn)
					}

					// Save Milestone
					db.Put(milestoneKey, tx.Timestamp, nil, txn)
				}
				requestIfMissing(tx.TrunkTransaction, "", txn)
				requestIfMissing(tx.BranchTransaction, "", txn)

				if tx != tipFastTX && (milestone || db.Has(db.GetByteKey(tx.Hash, db.KEY_UNKNOWN), txn)) {
					tx.confirm(txn)
				}

				// Re-broadcast new TX. Not always.
				go func () {
					if utils.Random(0,100) < 5 {
						outgoingQueue <- getMessage(*msg.Bytes, nil, false, txn)
					}
				}()
				server.NeighborTrackingQueue <- &server.NeighborTrackingMessage{Addr: msg.Addr, New: 1}
				saved++
			} else {
				discarded++
			}

			// Add request

			if len(requestReplyQueue) < 1000 {
				tipRequest := bytes.Equal(tx.Hash[:46], tipFastTX.Hash[:46]) || bytes.Equal(tx.Hash[:46], (*msg.Requested)[:46])
				req := make([]byte, 49)
				copy(req, *msg.Requested)
				requestReplyQueue <- &Request{req, msg.Addr, tipRequest}
			}

			return nil

		})
	}
}

func periodicRequest() {
	ticker := time.NewTicker(pingInterval)
	for range ticker.C {
		db.Locker.Lock()
		db.Locker.Unlock()
		_ = db.DB.View(func(txn *badger.Txn) error {
			// Request pending
			outgoingQueue <- getMessage(nil, nil, false, txn)
			return nil
		})
	}
}

func periodicTipRequest() {
	ticker := time.NewTicker(tipPingInterval)
	for range ticker.C {
		db.Locker.Lock()
		db.Locker.Unlock()
		_ = db.DB.View(func(txn *badger.Txn) error {
			// Request tip
			outgoingQueue <- getMessage(nil, nil, true, txn)
			return nil
		})
	}
}

func requestIfMissing (hash []byte, addr string, txn *badger.Txn) bool {
	tx := txn
	has := true
	if txn == nil {
		tx = db.DB.NewTransaction(true)
		defer func () error {
			return tx.Commit(func(e error) {})
		}()
	}
	if !db.Has(db.GetByteKey(hash, db.KEY_HASH), tx) { //&& !db.Has(db.GetByteKey(hash, db.KEY_REQUESTS), tx) {
		db.Put(db.GetByteKey(hash, db.KEY_REQUESTS), int(time.Now().Unix()), nil, tx)
		db.Put(db.GetByteKey(hash, db.KEY_REQUESTS_HASH), hash, nil, tx)
		message := getMessage(nil, hash, false, tx)
		message.Addr = addr
		outgoingQueue <- message
		has = false
	}
	return has
}

func report () {
	flushTicker := time.NewTicker(flushInterval)
	for range flushTicker.C {
		fmt.Printf("I: %v/%v O: %v/%v Saved: %v Discarded: %v \n", incomingProcessed, incoming, outgoingProcessed, outgoing, saved, discarded)
		fmt.Printf("Queues: SI I/O: %v/%v I: %v O: %v R: %v \n",
			len(srv.Incoming),
			len(srv.Outgoing),
			len(incomingQueue),
			len(outgoingQueue),
			len(requestReplyQueue))
		fmt.Printf("Totals: TXs %v/%v, Req: %v, Unknown: %v \n",
			db.Count(db.KEY_CONFIRMED),
			db.Count(db.KEY_TRANSACTION),
			db.Count(db.KEY_REQUESTS),
			db.Count(db.KEY_UNKNOWN))
	}
}
