package tangle

import (
	"db"
	"github.com/dgraph-io/badger"
	"server"
	"time"
	"convert"
)

func responseRunner () {
	for msg := range outgoingQueue {
		respond(msg)
	}
}

func replyToRequest (msg *Request, addr string, req []byte, txn *badger.Txn) {
	// Reply to requests:
	var resp []byte = nil

	if !msg.Tip {
		t, err := db.GetBytes(db.GetByteKey(msg.Requested, db.KEY_BYTES), txn)
		if err == nil {
			resp = t
		}
	}
	if msg.Tip || resp != nil {
		message := getMessage(resp, req, false, txn)
		message.Addr = addr
		outgoingQueue <- message
	} else {
		// If I do not have this TX, request from somewhere else?
		// TODO: (OPT) not always. Have a random drop ratio as in IRI.
		// requestIfMissing(msg.Requested, "", nil)
	}
}

func respond (msg *Message) {
	data := append((*msg.Bytes)[:1604], (*msg.Requested)[:46]...)
	srv.Outgoing <- &server.Message{msg.Addr, data}
}


func getMessage (tx []byte, req []byte, tip bool, txn *badger.Txn) *Message {
	var hash []byte
	// Try getting a weighted random tip (those with more value are preferred)
	if tx == nil {
		hash, tx = getRandomTip()
	}
	// Try getting latest milestone
	if tx == nil {
		milestone, ok := milestones[db.KEY_MILESTONE]
		if ok && milestone.TX != nil {
			tx = milestone.TX.Bytes
			if req == nil {
				hash = milestone.TX.Hash
			}
		}
	}
	// Otherwise, latest TX
	if tx == nil {
		key, _, _ := db.GetLatestKey(db.KEY_TIMESTAMP, txn)
		if key != nil {
			key = db.AsKey(key, db.KEY_BYTES)
			tx, _ = db.GetBytes(key, txn)
			if req == nil {
				key = db.AsKey(key, db.KEY_HASH)
				hash, _ = db.GetBytes(key, txn)
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

	// If no request (with a tx provided)
	if req == nil {
		// Select tip, if so requested, or one of the pending requests.
		if tip {
			req = hash
		} else {
			// TODO: remove this and find a way to load oldest from the DB?
			hashTrytes, _ := oldestPending()
			if len(hashTrytes) > 0 {
				req = convert.TrytesToBytes(hashTrytes)
				PendingsLocker.Lock()
				pendings[hashTrytes] = time.Now().UnixNano()
				PendingsLocker.Unlock()
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