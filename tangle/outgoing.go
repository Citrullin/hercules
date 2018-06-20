package tangle

import (
	"bytes"
	"encoding/gob"
	"time"

	"../db"
	"../logs"
	"../server"
	"github.com/dgraph-io/badger"
)

const (
	tipRequestInterval = time.Duration(200) * time.Millisecond
	reRequestInterval  = time.Duration(30) * time.Second
	maxIncoming        = 200
)

type PendingRequest struct {
	Hash             []byte
	Timestamp        int
	LastTried        time.Time
	LastNeighborAddr string
}

var lastTip = time.Now()
var pendingRequests []*PendingRequest
var rotatePending = 0

func Broadcast(data []byte) int {
	sent := 0

	server.NeighborsLock.RLock()
	for _, neighbor := range server.Neighbors {
		server.NeighborsLock.RUnlock()

		request := getSomeRequest(neighbor.Addr)
		sendReply(getMessage(data, request, request == nil, neighbor.Addr, nil))
		sent++

		server.NeighborsLock.RLock()
	}
	server.NeighborsLock.RUnlock()

	return sent
}

func pendingOnLoad() {
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
					for _, neighbor := range server.Neighbors {
						queue, ok := requestQueues[neighbor.Addr]
						if !ok {
							q := make(RequestQueue, maxQueueSize)
							queue = &q
							requestQueues[neighbor.Addr] = queue
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

func getSomeRequest(addr string) []byte {
	requestLocker.RLock()
	requestQueue, requestOk := requestQueues[addr]
	requestLocker.RUnlock()
	var request []byte
	if requestOk && len(*requestQueue) > 0 {
		request = (<-*requestQueue).Requested
	}
	if request == nil {
		pendingRequest := getOldPending()
		if pendingRequest != nil {
			pendingRequest.LastTried = time.Now()
			pendingRequest.LastNeighborAddr = addr
			request = pendingRequest.Hash
		}
	}
	return request
}

func outgoingRunner() {
	if len(srv.Incoming) > maxIncoming {
		return
	}

	shouldRequestTip := false
	if lowEndDevice {
		shouldRequestTip = time.Now().Sub(lastTip) > tipRequestInterval*5
	} else {
		shouldRequestTip = time.Now().Sub(lastTip) > tipRequestInterval
	}

	server.NeighborsLock.RLock()
	for _, neighbor := range server.Neighbors {
		server.NeighborsLock.RUnlock()

		var request = getSomeRequest(neighbor.Addr)
		if request != nil {
			sendReply(getMessage(nil, request, false, neighbor.Addr, nil))
		} else if len(srv.Incoming) < 50 && shouldRequestTip {
			lastTip = time.Now()
			sendReply(getMessage(nil, nil, true, neighbor.Addr, nil))
		}

		server.NeighborsLock.RLock()
	}
	server.NeighborsLock.RUnlock()
}

func requestIfMissing(hash []byte, addr string, txn *badger.Txn) (has bool, err error) {
	tx := txn
	has = true
	if bytes.Equal(hash, tipFastTX.Hash) {
		return has, nil
	}
	if txn == nil {
		tx = db.DB.NewTransaction(true)
		defer func() error {
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

		pending := addPendingRequest(hash, timestamp, addr)
		if pending != nil {
			requestLocker.Lock()
			_, neighbor := server.GetNeighborByAddress(addr)
			if neighbor != nil {
				queue, ok := requestQueues[neighbor.Addr]
				if !ok {
					q := make(RequestQueue, maxQueueSize)
					queue = &q
					requestQueues[neighbor.Addr] = queue
				}
				requestLocker.Unlock()
				*queue <- &Request{hash, false}
			}
		}

		has = false
	}
	return has, nil
}

func sendReply(msg *Message) {
	if msg == nil {
		return
	}
	data := append((*msg.Bytes)[:1604], (*msg.Requested)[:46]...)
	srv.Outgoing <- &server.Message{Addr: msg.Addr, Msg: data}
	outgoing++
}

func getMessage(resp []byte, req []byte, tip bool, addr string, txn *badger.Txn) *Message {
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
	queue, ok := requestQueues[addr]
	if !ok {
		q := make(RequestQueue, maxQueueSize)
		queue = &q
		requestQueues[addr] = queue
	}
	*queue <- &Request{pendingRequest.Hash, false}
	pendingRequest.LastTried = time.Now()
}

func addPendingRequest(hash []byte, timestamp int, addr string) *PendingRequest {
	pendingRequestLocker.Lock()
	defer pendingRequestLocker.Unlock()

	var which = findPendingRequest(hash)
	if which >= 0 {
		return nil
	} // Avoid double-add

	pendingRequest := &PendingRequest{hash, timestamp, time.Now(), addr}
	pendingRequests = append(pendingRequests, pendingRequest)
	return pendingRequest
}

func removePendingRequest(hash []byte) bool {
	pendingRequestLocker.Lock()
	defer pendingRequestLocker.Unlock()
	var which = findPendingRequest(hash)
	if which > -1 {
		if which >= len(pendingRequests)-1 {
			pendingRequests = pendingRequests[0:which]
		} else {
			pendingRequests = append(pendingRequests[0:which], pendingRequests[which+1:]...)
		}
		return true
	}
	return false
}

func findPendingRequest(hash []byte) int {
	for i, pendingRequest := range pendingRequests {
		if bytes.Equal(hash, pendingRequest.Hash) {
			return i
		}
	}
	return -1
}

func getOldPending() *PendingRequest {
	pendingRequestLocker.RLock()
	defer pendingRequestLocker.RUnlock()
	sortRange := 2000
	if lowEndDevice {
		sortRange = 500
	}
	for i, pendingRequest := range pendingRequests {
		if time.Now().Sub(pendingRequest.LastTried) > reRequestInterval {
			return pendingRequest
		}
		if i >= sortRange {
			return nil
		}
	}

	rotatePending++
	if rotatePending > 5000 && len(pendingRequests) > sortRange {
		pendingRequests = append(pendingRequests[sortRange:], pendingRequests[:sortRange]...)
		rotatePending = 0
	}
	return nil
}
