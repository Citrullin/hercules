package tangle

import (
	"db"
	"github.com/dgraph-io/badger"
	"logs"
	"server"
	"time"
	"bytes"
	"encoding/gob"
	"math"
)

const (
	tipRequestInterval = 10
	reRequestInterval = time.Duration(5) * time.Second
)

type PendingRequest struct {
	Hash       []byte
	Timestamp  int
	LastTried  time.Time
	LastNeighborAddr string
}

var lastTip = time.Now()
var pendingRequests []*PendingRequest

func pendingOnLoad () {
	loadPendingRequests()
}

func loadPendingRequests() {
	logs.Log.Info("Loading pending requests")

	db.Locker.Lock()
	defer db.Locker.Unlock()
	requestLocker.Lock()
	total := 0
	added := 0

	defer requestLocker.Unlock()
	_ = db.DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte{db.KEY_PENDING_HASH}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			total++
			item := it.Item()
			v, _ := item.Value()
			var hash []byte
			buf := bytes.NewBuffer(v)
			dec := gob.NewDecoder(buf)
			err := dec.Decode(&hash)
			if err == nil {
				timestamp, err := db.GetInt(db.AsKey(item.Key(), db.KEY_PENDING_TIMESTAMP), txn)
				if err == nil {
					for addr := range server.Neighbors {
						queue, ok := requestQueues[addr]
						if !ok {
							q := make(RequestQueue, maxQueueSize)
							queue = &q
							requestQueues[addr] = queue
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
		}
		return nil
	})

	logs.Log.Info("Pending requests loaded", added, total)
}

func outgoingRunner() {
	tInterval := int(math.Max(float64(server.Speed) / 100 * tipRequestInterval, tipRequestInterval))
	shouldRequestTip := time.Now().Sub(lastTip) > time.Duration(tInterval) * time.Millisecond
	for neighbor := range server.Neighbors {
		requestLocker.RLock()
		requestQueue, requestOk := requestQueues[neighbor]
		requestLocker.RUnlock()
		replyLocker.RLock()
		replyQueue, replyOk := replyQueues[neighbor]
		replyLocker.RUnlock()
		var request []byte
		var reply []byte
		if requestOk && len(*requestQueue) > 0 {
			request = (<-*requestQueue).Requested
		}
		if replyOk && len(*replyQueue) > 0 {
			reply, _ = db.GetBytes(db.GetByteKey((<-*replyQueue).Requested, db.KEY_BYTES), nil)
		}
		if request != nil || reply != nil {
			msg := getMessage(reply, request, request == nil, neighbor, nil)
			msg.Addr = neighbor
			sendReply(msg)
		} else if shouldRequestTip {
			lastTip = time.Now()
			msg := getMessage(nil, nil, true, neighbor,nil)
			msg.Addr = neighbor
			sendReply(msg)
		} else {
			pendingRequest := getOldPending()
			if pendingRequest != nil && pendingRequest.LastNeighborAddr != neighbor {
				request = pendingRequest.Hash
				pendingRequest.LastTried = time.Now()
				pendingRequest.LastNeighborAddr = neighbor
			}
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
		queue, ok := requestQueues[addr]
		if !ok {
			q := make(RequestQueue, maxQueueSize)
			queue = &q
			requestQueues[addr] = queue
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
	queue, ok := requestQueues[addr]
	if !ok {
		q := make(RequestQueue, maxQueueSize)
		queue = &q
		requestQueues[addr] = queue
	}
	*queue <- &Request{pendingRequest.Hash, false}
	pendingRequest.LastTried = time.Now()
}

func addPendingRequest (hash []byte, timestamp int, addr string) *PendingRequest {
	pendingRequest := &PendingRequest{hash, timestamp,time.Now(), addr}
	pendingRequestLocker.Lock()
	pendingRequests = append(pendingRequests, pendingRequest)
	pendingRequestLocker.Unlock()
	return pendingRequest
}

func removePendingRequest (hash []byte) bool {
	var which = -1
	pendingRequestLocker.Lock()
	defer pendingRequestLocker.Unlock()
	for i, pendingRequest := range pendingRequests {
		if bytes.Equal(hash, pendingRequest.Hash) {
			which = i
		}
	}
	if which > -1 {
		pendingRequests = append(pendingRequests[0:which], pendingRequests[which+1:]...)
		return true
	}
	return false
}

func getOldPending () *PendingRequest{
	pendingRequestLocker.RLock()
	defer pendingRequestLocker.RUnlock()
	for _, pendingRequest := range pendingRequests {
		if time.Now().Sub(pendingRequest.LastTried) > reRequestInterval {
			return pendingRequest
		}
	}
	return nil
}
