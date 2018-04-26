package tangle

import (
	"fmt"
	"time"
	"runtime"
	"db"
	"convert"
	"github.com/dgraph-io/badger"
	"strings"
)

const (
	maxQueueSize = 1000000
	maxRequestQueueSize = 10000
	flushInterval = time.Duration(10) * time.Second
	pingInterval = time.Duration(500) * time.Millisecond
	cooAddress = "KPWCHICGJZXKE9GSUDXZYUAPLHAKAHYHDXNPHENTERYMMBQOPSQIDENXKLKCEYCPVTZQLEEJVYJZV9BWU"
)

type Message struct {
	Trytes *[]byte
	Requested *[]byte
	Addr string
}

type Request struct {
	Requested string
	Addr string
	Tip bool
}

type RequestQueue chan *Request
type messageQueue chan *Message

type Tangle struct {
	Incoming messageQueue
	Outgoing messageQueue
}

var nbWorkers = runtime.NumCPU()
var tangle *Tangle
var requestQueue RequestQueue
var latestMilestoneKey = []byte("MilestoneLatest")

// Remove temporary: use global through iterator of DB
var received = 0
var confirmed = 0
var replied = 0

func Start () *Tangle {
	tangle = &Tangle{
		Incoming: make(messageQueue, maxQueueSize),
		Outgoing: make(messageQueue, maxQueueSize)}
	requestQueue = make(RequestQueue, maxRequestQueueSize)
	go periodicRequest()
	go periodicResponse()
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
		_ = db.DB.Update(func(txn *badger.Txn) error {
			trytes := convert.BytesToTrytes(*msg.Trytes)[:2673]
			tx := TrytesToObject(trytes)
			db.Remove(db.GetHashKey(tx.Hash, db.KEY_REQUESTS), txn)
			db.Remove(db.GetHashKey(tx.Hash, db.KEY_REQUESTS_HASH), txn)
			milestone := isMilestone(tx)

			// TODO: check if the TX is recent (than snapshot). Otherwise drop.

			if !db.Has(db.GetHashKey(tx.Hash, db.KEY_HASH), txn) {
				trunkBytes := convert.TrytesToBytes(tx.TrunkTransaction)[:49]
				branchBytes := convert.TrytesToBytes(tx.BranchTransaction)[:49]
				trunk := db.GetByteKey(trunkBytes, db.KEY_TRANSACTION)
				branch := db.GetByteKey(branchBytes, db.KEY_TRANSACTION)
				both := make([]byte, 32)
				copy(both, trunk)
				copy(both[16:], branch)

				hashBytes := convert.TrytesToBytes(tx.Hash)
				db.Put(db.GetHashKey(tx.Hash, db.KEY_HASH), hashBytes, txn)
				db.Put(db.GetHashKey(tx.Hash, db.KEY_TIMESTAMP), tx.Timestamp, txn)
				db.Put(db.GetHashKey(tx.Hash, db.KEY_TRANSACTION), *msg.Trytes, txn)
				//db.Put(db.GetHashKey(tx.Hash, db.KEY_RELATION), both, txn)
				if milestone {
					milestoneKey := db.GetHashKey(tx.Hash, db.KEY_MILESTONE)

					// Update the latest milestone saved
					_, timestamp, err := db.GetLatestKey(db.KEY_MILESTONE, txn)
					if err != nil || timestamp < tx.Timestamp {
						db.Put(latestMilestoneKey, milestoneKey, txn)
					}

					// Save Milestone
					db.Put(milestoneKey, tx.Timestamp, txn)
				}

				if !db.Has(db.GetHashKey(tx.TrunkTransaction, db.KEY_HASH), txn) && !db.Has(db.GetHashKey(tx.TrunkTransaction, db.KEY_REQUESTS), txn) {
					db.Put(db.GetHashKey(tx.TrunkTransaction, db.KEY_REQUESTS), int(time.Now().Unix()), txn)
					db.Put(db.GetHashKey(tx.TrunkTransaction, db.KEY_REQUESTS_HASH), trunkBytes, txn)
					message := getMessage(nil, trunkBytes, false, txn)
					//message.Addr = msg.Addr
					tangle.Outgoing <- message
				}
				if !db.Has(db.GetHashKey(tx.BranchTransaction, db.KEY_HASH), txn) && !db.Has(db.GetHashKey(tx.BranchTransaction, db.KEY_REQUESTS), txn) {
					db.Put(db.GetHashKey(tx.TrunkTransaction, db.KEY_REQUESTS), int(time.Now().Unix()), txn)
					db.Put(db.GetHashKey(tx.TrunkTransaction, db.KEY_REQUESTS_HASH), branchBytes, txn)
					message := getMessage(nil, branchBytes, false, txn)
					//message.Addr = msg.Addr
					tangle.Outgoing <- message
				}
				// Re-broadcast new TX
				tangle.Outgoing <- getMessage(*msg.Trytes, nil, false, txn)
				received++
			}

			rTrytes := convert.BytesToTrytes(*msg.Requested) + strings.Repeat("9", 4)
			isTip := rTrytes == tx.Hash || rTrytes == strings.Repeat("9", 81)
			requestQueue <- &Request{rTrytes, msg.Addr, isTip}

			if milestone || db.Has(db.GetHashKey(tx.Hash, db.KEY_UNKNOWN), txn) {
				go startConfirm(tx)
			}

			return nil
		})
	}
}

func periodicResponse() {
	for msg := range requestQueue {
		replyToRequest(msg)
	}
}

func replyToRequest (msg *Request) {
	db.Locker.Lock()
	db.Locker.Unlock()
	_ = db.DB.Update(func(txn *badger.Txn) error {
		// Reply to requests:
		var resp []byte = nil

		if !msg.Tip {
			t, err := db.GetBytes(db.GetHashKey(msg.Requested, db.KEY_TRANSACTION), txn)
			resp = t
			if err != nil { replied++ }
		}
		if msg.Tip || resp != nil {
			message := getMessage(resp, nil, false, txn)
			message.Addr = msg.Addr
			tangle.Outgoing <- message
			// If I do not have this TX, request from somewhere else?
		} else if !msg.Tip && resp == nil {
			// TODO: not always. Have a random drop ratio as in IRI.
			db.Put(db.GetHashKey(msg.Requested, db.KEY_REQUESTS), int(time.Now().Unix()), txn)
			db.Put(db.GetHashKey(msg.Requested, db.KEY_REQUESTS_HASH), convert.TrytesToBytes(msg.Requested)[:49], txn)
			tangle.Outgoing <- getMessage(nil, convert.TrytesToBytes(msg.Requested), false, txn)
		}
		replied++
		return nil
	})
}

func startConfirm (tx *TX) {
	db.Locker.Lock()
	db.Locker.Unlock()
	_ = db.DB.Update(func(txn *badger.Txn) error {
		confirm(tx, txn)
		return nil
	})
}

func confirm (tx *TX, txn *badger.Txn) {
	db.Remove(db.GetHashKey(tx.Hash, db.KEY_UNKNOWN), txn)
	db.Put(db.GetHashKey(tx.Hash, db.KEY_CONFIRMED), tx.Timestamp, txn)
	confirmed++
	if tx.Value > 0 {
		_, err := db.IncrBy(db.GetHashKey(tx.Hash, db.KEY_ACCOUNT), tx.Value, true, txn)
		if err != nil {
			panic("Could not update account balance!")
		}
	}
	confirmChild(tx.TrunkTransaction, txn)
	confirmChild(tx.BranchTransaction, txn)
}

func confirmChild (hash string, txn *badger.Txn) {
	if db.Has(db.GetHashKey(hash, db.KEY_CONFIRMED), txn) { return }
	if db.Has(db.GetHashKey(hash, db.KEY_TRANSACTION), txn) {
		var bytes []byte
		t, err := db.GetBytes(db.GetHashKey(hash, db.KEY_TRANSACTION), txn)
		if err != nil {
			bytes = t
			tx := TrytesToObject(convert.BytesToTrytes(bytes[:1604])[:2673])
			if tx != nil {
				confirm(tx, txn)
			}
		}
	} else {
		db.Put(db.GetHashKey(hash, db.KEY_UNKNOWN), int(time.Now().Unix()), txn)
		db.Put(db.GetHashKey(hash, db.KEY_REQUESTS), int(time.Now().Unix()), txn)
		db.Put(db.GetHashKey(hash, db.KEY_REQUESTS_HASH), convert.TrytesToBytes(hash)[:49], txn)
		tangle.Outgoing <- getMessage(nil, convert.TrytesToBytes(hash), false, txn)
	}

}

func isMilestone(tx *TX) bool {
	// TODO: check if really milestone
	return tx.Address == cooAddress
}

func periodicRequest() {
	ticker := time.NewTicker(pingInterval)
	for range ticker.C {
		db.Locker.Lock()
		db.Locker.Unlock()
		_ = db.DB.View(func(txn *badger.Txn) error {
			// Request tip
			tangle.Outgoing <- getMessage(nil, nil, true, txn)
			// Request pending
			tangle.Outgoing <- getMessage(nil, nil, false, txn)
			return nil
		})
	}
}

func report () {
	flushTicker := time.NewTicker(flushInterval)
	for range flushTicker.C {
		fmt.Printf("TXs: %v/%v, Queue: %v Pending: %v Replied: %v \n", confirmed, received, len(tangle.Incoming), db.Count(db.KEY_REQUESTS), replied)
	}
}

func getMessage (tx []byte, req []byte, tip bool, txn *badger.Txn) *Message {
	var hash []byte
	// Try getting latest milestone
	if tx == nil {
		key, _, _ := db.GetLatestKey(db.KEY_MILESTONE, txn)
		if key != nil {
			key[0] = db.KEY_TRANSACTION
			t, _ := db.GetBytes(key, txn)
			tx = t
			if req == nil {
				key[0] = db.KEY_HASH
				t, _ := db.GetBytes(key, txn)
				hash = t
			}
		}
	}
	// Otherwise, latest TX
	if tx == nil {
		key, _, _ := db.GetLatestKey(db.KEY_TIMESTAMP, txn)
		if key != nil {
			key[0] = db.KEY_TRANSACTION
			t, _ := db.GetBytes(key, txn)
			tx = t
			if req == nil {
				key[0] = db.KEY_HASH
				t, _ := db.GetBytes(key, txn)
				hash = t
			}
		}
	}
	// Random
	if tx == nil {
		tx = make([]byte, 1604)
		if req == nil {
			hash = make([]byte, 46)
		}
	}

	// If no request
	if req == nil {
		// Select tip, if so requested, or one of the pending requests.
		if tip {
			req = hash
		} else {
			key := db.PickRandomKey(db.KEY_REQUESTS_HASH, txn)
			if key != nil {
				key[0] = db.KEY_REQUESTS_HASH
				t, _ := db.GetBytes(key, txn)
				req = t
			} else {
				// If tip=false and no pending, force tip=true
				req = hash
			}
		}
	}
	if req == nil {
		req = make([]byte, 46)
	}
	return &Message{&tx, &req, ""}
}
