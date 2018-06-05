package tangle

import (
	"db"
	"github.com/dgraph-io/badger"
	"logs"
	"server"
	"time"
	"bytes"
	"encoding/gob"
)

const (
	tipRequestInterval = time.Duration(3) * time.Second
	reRequestInterval = time.Duration(30) * time.Second
)

type PendingRequest struct {
	Hash       []byte
	Timestamp  int
	LastTried  time.Time
	LastNeighborAddr string
}

var lastTip = time.Now()
var pendingRequests []*PendingRequest
var rotatePending = 0

func pendingOnLoad () {
	loadPendingRequests()
}

func loadPendingRequests() {
	// TODO: if pending is pending for too long, remove it from the loop?
	logs.Log.Info("Loading pending requests")

	db.Locker.Lock()
	defer db.Locker.Unlock()
	requestLocker.Lock()
	defer requestLocker.Unlock()

	total := 0
	added := 0

	_ = db.DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte{db.KEY_PENDING_HASH}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			v, _ := item.Value()
			var hash []byte
			buf := bytes.NewBuffer(v)
			dec := gob.NewDecoder(buf)
			err := dec.Decode(&hash)
			if err == nil {
				timestamp, err := db.GetInt(db.AsKey(item.Key(), db.KEY_PENDING_TIMESTAMP), txn)
				if err == nil {
					for identifier := range server.Neighbors {
						queue, ok := requestQueues[identifier]
						if !ok {
							q := make(RequestQueue, maxQueueSize)
							queue = &q
							requestQueues[identifier] = queue
						}
						*queue <- &Request{hash, false}
					}
					addPendingRequest(hash, timestamp, "")
					added++
				} else {
					logs.Log.Warning("Could not load pending Tx Timestamp")
				}
			} else {
				logs.Log.Warning("Could not load pending Tx Hash")
			}
			total++
		}
		return nil
	})

	logs.Log.Info("Pending requests loaded", added, total)
}

func outgoingRunner() {
	if len(txQueue) > 100 { return }
	var pendingRequest *PendingRequest
	shouldRequestTip := time.Now().Sub(lastTip) > tipRequestInterval

	for identifier, neighbor := range server.Neighbors {
		requestLocker.RLock()
		requestQueue, requestOk := requestQueues[identifier]
		requestLocker.RUnlock()
		replyLocker.RLock()
		replyQueue, replyOk := replyQueues[identifier]
		replyLocker.RUnlock()
		var request []byte
		var reply []byte
		if requestOk && len(*requestQueue) > 0 {
			request = (<-*requestQueue).Requested
		}
		if replyOk && len(*replyQueue) > 0 {
			reply, _ = db.GetBytes(db.GetByteKey((<-*replyQueue).Requested, db.KEY_BYTES), nil)
		}
		if request == nil {
			pendingRequest = getOldPending()
			if pendingRequest != nil {
				pendingRequest.LastTried = time.Now()
				pendingRequest.LastNeighborAddr = identifier
				request = pendingRequest.Hash
			}
		}
		if request != nil || reply != nil {
			msg := getMessage(reply, request, request == nil, identifier, nil)
			msg.Addr = identifier
			sendReply(msg)
		} else if len(srv.Incoming) < 50 {
			pendingRequest = getOldPending()
			if pendingRequest != nil && pendingRequest.LastNeighborAddr != neighbor.Addr {
				pendingRequest.LastTried = time.Now()
				pendingRequest.LastNeighborAddr = identifier
				msg := getMessage(nil, pendingRequest.Hash, false, identifier, nil)
				msg.Addr = neighbor.Addr
				sendReply(msg)
			} else if shouldRequestTip {
				lastTip = time.Now()
				msg := getMessage(nil, nil, true, identifier,nil)
				msg.Addr = neighbor.Addr
				sendReply(msg)
			}
		}
	}
}

func requestIfMissing (hash []byte, addr string, txn *badger.Txn) (has bool, err error) {
	tx := txn
	has = true
	if bytes.Equal(hash, tipFastTX.Hash) {
		return has, nil
	}
	if txn == nil {
		tx = db.DB.NewTransaction(true)
		defer func () error {
			return tx.Commit(func(e error) {})
		}()
	}
	key := db.GetByteKey(hash, db.KEY_HASH)
	if !db.Has(key, tx) && !db.Has(db.AsKey(key, db.KEY_PENDING_TIMESTAMP), tx) {
		timestamp := int(time.Now().Unix())
		err := db.Put(db.AsKey(key, db.KEY_PENDING_HASH), hash, nil, txn)
		err2 := db.Put(db.AsKey(key, db.KEY_PENDING_TIMESTAMP), timestamp, nil, txn)

		if err != nil {
			logs.Log.Error("Failed saving new TX request", err)
			return false, err
		}

		if err2 != nil {
			logs.Log.Error("Failed saving new TX request", err)
			return false, err
		}

		requestLocker.Lock()
		identifier, _ := server.GetNeighborByAddress(addr)
		queue, ok := requestQueues[identifier]
		if !ok {
			q := make(RequestQueue, maxQueueSize)
			queue = &q
			requestQueues[identifier] = queue
		}
		requestLocker.Unlock()
		*queue <- &Request{hash, false}
		addPendingRequest(hash, timestamp, addr)

		has = false
	}
	return has, nil
}

func sendReply (msg *Message) {
	if msg == nil { return }
	data := append((*msg.Bytes)[:1604], (*msg.Requested)[:46]...)
	srv.Outgoing <- &server.Message{msg.Addr, data}
	outgoing++
}

func getMessage (resp []byte, req []byte, tip bool, addr string, txn *badger.Txn) *Message {
	var hash []byte
	if resp == nil {
		hash, resp = getRandomTip()
	}
	// Try getting latest milestone
	if resp == nil {
		milestone := LatestMilestone
		if milestone.TX != nil && milestone.TX != tipFastTX {
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
			pendingRequest := getOldPending()
			if pendingRequest != nil {
				req = pendingRequest.Hash
				pendingRequest.LastTried = time.Now()
				pendingRequest.LastNeighborAddr = addr
			}
		}
	}
	if req == nil {
		req = hash
	}
	if req == nil {
		req = make([]byte, 46)
	}
	return &Message{&resp, &req, addr}
}

func (pendingRequest PendingRequest) request(addr string) {
	identifier, _ := server.GetNeighborByAddress(addr)
	queue, ok := requestQueues[identifier]
	if !ok {
		q := make(RequestQueue, maxQueueSize)
		queue = &q
		requestQueues[identifier] = queue
	}
	*queue <- &Request{pendingRequest.Hash, false}
	pendingRequest.LastTried = time.Now()
}

func addPendingRequest (hash []byte, timestamp int, addr string) *PendingRequest {
	pendingRequestLocker.Lock()
	defer pendingRequestLocker.Unlock()

	var which = findPendingRequest(hash)
	if which >= 0 { return nil } // Avoid double-add

	pendingRequest := &PendingRequest{hash, timestamp,time.Now(), addr}
	pendingRequests = append(pendingRequests, pendingRequest)
	return pendingRequest
}

func removePendingRequest (hash []byte) bool {
	pendingRequestLocker.Lock()
	defer pendingRequestLocker.Unlock()
	var which = findPendingRequest(hash)
	if which > -1 {
		if which >= len(pendingRequests) - 1 {
			pendingRequests = pendingRequests[0:which]
		} else {
			pendingRequests = append(pendingRequests[0:which], pendingRequests[which+1:]...)
		}
		return true
	}
	return false
}

func findPendingRequest (hash []byte) int {
	for i, pendingRequest := range pendingRequests {
		if bytes.Equal(hash, pendingRequest.Hash) {
			return i
		}
	}
	return -1
}

func getOldPending () *PendingRequest{
	pendingRequestLocker.RLock()
	defer pendingRequestLocker.RUnlock()
	for i, pendingRequest := range pendingRequests {
		if time.Now().Sub(pendingRequest.LastTried) > reRequestInterval {
			return pendingRequest
		}
		// TODO: OK to put this limit? Make it less for low-end devices
		if i >= 5000 { return nil }
	}
	rotatePending++
	if rotatePending > 1000 && len(pendingRequests) > 5000 {
		pendingRequests = append(pendingRequests[5000:], pendingRequests[:5000]...)
		rotatePending = 0
	}
	return nil
}

func Broadcast(hash []byte) int {
	queued := 0
	replyLocker.RLock()
	for _, queue := range replyQueues {
		if len(*queue) < 1000 {
			*queue <- &Request{hash, false}
			queued++
		}
	}
	replyLocker.RUnlock()
	return queued
}
