package tangle

import (
	"time"
	"db"
	"github.com/dgraph-io/badger"
	"logs"
	"server"
)

func periodicRequest() {
	ticker := time.NewTicker(pingInterval)
	for range ticker.C {
		request(nil, "", false)
	}
}

func periodicTipRequest() {
	ticker := time.NewTicker(tipPingInterval)
	for range ticker.C {
		request(nil, "", true)
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
		err2 := db.Put(db.GetByteKey(hash, db.KEY_PENDING_HASH), hash, nil, nil)

		if err2 != nil {
			logs.Log.Error("Failed saving new TX request", err2)
			return has, err2
		}

		request(hash, addr,false)
		has = false
	}
	return has, nil
}

func request (hash []byte, addr string, tip bool) {
	var resp []byte
	var ok = false
	var queue *RequestQueue

	if len(addr) > 0 {
		queue, ok = requestReplyQueues[addr]
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
				t, err := db.GetBytes(db.GetByteKey(request.Requested, db.KEY_BYTES), nil)
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

	msg := getMessage(nil, hash, tip, nil)
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
	// Otherwise, latest TX
	if resp == nil {
		key, _, _ := db.GetLatestKey(db.KEY_TIMESTAMP, txn)
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
			key := db.PickRandomKey(db.KEY_PENDING_HASH, txn)
			if key != nil {
				key = db.AsKey(key, db.KEY_PENDING_HASH)
				t, _ := db.GetBytes(key, txn)
				req = t
			} else {
				// If tip=false and no pending, force tip=true
				req = hash
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