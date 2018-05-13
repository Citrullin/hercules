package tangle

import (
	"db"
	"github.com/dgraph-io/badger"
	"logs"
	"server"
	"time"
	"bytes"
)

const (
	tipRequestInterval = time.Duration(500) * time.Millisecond
)

var lastTip = time.Now()

func outgoingRunner() {
	for addr, queue := range requestQueues {
		select {
		case msg := <- *queue:
			request(msg.Requested, addr, false)
		default:
			now := time.Now()
			tip := false
			if now.Sub(lastTip) > tipRequestInterval {
				tip = true
				lastTip = now
			}
			request(nil, addr, tip)
		}
	}
}

func requestIfMissing (hash []byte, addr string, txn *badger.Txn) (has bool, err error) {
	tx := txn
	has = true
	if txn == nil {
		tx = db.DB.NewTransaction(true)
		defer func () error {
			return tx.Commit(func(e error) {})
		}()
	}
	if !db.Has(db.GetByteKey(hash, db.KEY_HASH), tx) { //&& !db.Has(db.GetByteKey(hash, db.KEY_PENDING), tx) {
		err := db.Put(db.GetByteKey(hash, db.KEY_PENDING_HASH), hash, nil, nil)

		if err != nil {
			logs.Log.Error("Failed saving new TX request", err)
			return false, err
		}

		pendingLocker.Lock()
		pendingHashes = append(pendingHashes, hash)
		pendingLocker.Unlock()

		queue, ok := requestQueues[addr]
		if !ok {
			q := make(RequestQueue, maxQueueSize)
			queue = &q
			requestQueues[addr] = queue
		}
		*queue <- &Request{hash, false}

		has = false
	}
	return has, nil
}

func request (hash []byte, addr string, tip bool) {
	txn := db.DB.NewTransaction(false)
	defer txn.Commit(func(e error) {})

	var resp []byte
	var ok = false
	var queue *RequestQueue

	if len(addr) > 0 {
		queue, ok = replyQueues[addr]
	}

	if ok {
		var request *Request = nil
		select {
		case msg := <- *queue:
			request = msg
		default:
			request = nil
		}
		if request != nil {
			if !request.Tip {
				t, err := db.GetBytes(db.GetByteKey(request.Requested, db.KEY_BYTES), txn)
				if err == nil {
					resp = t
				}
			}
			if !request.Tip && resp == nil {
				// If I do not have this TX, request from somewhere else?
				// TODO: (OPT) not always. Have a random drop ratio as in IRI.
				// requestIfMissing(msg.Requested, "", nil)
			}
		}
	}

	msg := getMessage(resp, hash, tip, txn)
	if msg == nil { return }
	data := append((*msg.Bytes)[:1604], (*msg.Requested)[:46]...)
	srv.Outgoing <- &server.Message{addr, data}
}

func getMessage (resp []byte, req []byte, tip bool, txn *badger.Txn) *Message {
	var hash []byte
	// Try getting a weighted random tip (those with more value are preferred)
	if resp == nil {
		hash, resp = getRandomTip()
	}
	// Try getting latest milestone
	if resp == nil {
		milestone, ok := milestones[db.KEY_MILESTONE]
		if ok && milestone.TX != nil {
			resp = milestone.TX.Bytes
			if req == nil {
				hash = milestone.TX.Hash
			}
		}
	}
	// Otherwise, latest (youngest) TX
	if resp == nil {
		key, _, _ := db.GetLatestKey(db.KEY_TIMESTAMP, false, txn)
		if key != nil {
			key = db.AsKey(key, db.KEY_BYTES)
			resp, _ = db.GetBytes(key, txn)
			if req == nil {
				key = db.AsKey(key, db.KEY_HASH)
				hash, _ = db.GetBytes(key, txn)
			}
		}
	}
	// Random
	if resp == nil {
		resp = make([]byte, 1604)
		if req == nil {
			hash = make([]byte, 46)
		}
	}

	// If no request provided
	if req == nil {
		// Select tip, if so requested, or one of the random pending requests.
		if tip {
			req = hash
		} else {
			if pendingHashes != nil && len(pendingHashes) > 0 {
				pendingLocker.Lock()
				req := pendingHashes[0]
				pendingHashes = append(pendingHashes[1:], req)
				pendingLocker.Unlock()
			} else {
				return nil
			}
		}
	}
	if req == nil {
		req = hash
	}
	if req == nil {
		req = make([]byte, 46)
	}
	return &Message{&resp, &req, ""}
}

func removePendingHash (hash []byte) {
	pendingLocker.Lock()
	defer pendingLocker.Unlock()
	if pendingHashes == nil {
		return
	}
	var hashes [][]byte
	for _, pending := range pendingHashes {
		if !bytes.Equal(pending, hash) {
			hashes = append(hashes, pending)
		}
	}
	pendingHashes = hashes
}