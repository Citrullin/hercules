package tangle

import (
	"db"
	"github.com/dgraph-io/badger"
	"bytes"
	"server"
)

func responseRunner () {
	for msg := range outgoingQueue {
		respond(msg)
	}
}

func requestReplyRunner() {
	for msg := range requestReplyQueue {
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
			t, err := db.GetBytes(db.GetByteKey(msg.Requested, db.KEY_TRANSACTION), txn)
			if err == nil {
				resp = t
			}
		}
		if msg.Tip || resp != nil {
			message := getMessage(resp, nil, false, txn)
			message.Addr = msg.Addr
			outgoingQueue <- message
		} else {
			// If I do not have this TX, request from somewhere else?
			// TODO: not always. Have a random drop ratio as in IRI.
			requestIfMissing(msg.Requested, "", txn)
		}
		return nil
	})
}

func respond (msg *Message) {
	db.Locker.Lock()
	db.Locker.Unlock()
	_ = db.DB.Update(func(txn *badger.Txn) error {
		outgoing++
		var fingerprint []byte
		var has = false
		var tipRequest = bytes.Equal(*msg.Requested, tipFastTX.Hash[:46])

		if !tipRequest {
			fingerprint = db.GetByteKey(append(*msg.Requested, msg.Addr...), db.KEY_FINGERPRINT)
			if db.Has(fingerprint, txn) {
				has = true
			} else {
				db.Put(fingerprint, true, &reRequestTTL, txn)
			}
		}
		if !has {
			outgoingProcessed++
			data := append((*msg.Bytes)[:1604], (*msg.Requested)[:46]...)
			srv.Outgoing <- &server.Message{msg.Addr, data}
		}
		return nil
	})
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
		req = hash
	}
	if req == nil {
		req = make([]byte, 46)
	}
	return &Message{&tx, &req, ""}
}