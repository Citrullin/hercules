package tangle

import (
	"bytes"
	"sync"
	"time"

	"../db"
	"../db/coding"
	"../logs"
	"../server"
	"../utils"
	"github.com/lukechampine/randmap"
)

const (
	tipRequestInterval = time.Duration(200) * time.Millisecond
	reRequestInterval  = time.Duration(10) * time.Second
	maxIncoming        = 100
	maxTimesRequest    = 100
)

type PendingRequest struct {
	Hash             []byte
	Timestamp        int
	LastTried        time.Time
	LastNeighborAddr string
}

var lastTip = time.Now()
var PendingRequests map[string]*PendingRequest
var PendingRequestsLock = &sync.RWMutex{}

func Broadcast(data []byte, exclude string) int {
	sent := 0

	var neighborsTmp = make(map[string]string)
	server.NeighborsLock.RLock()
	for _, neighbor := range server.Neighbors {
		neighborsTmp[neighbor.Addr] = neighbor.IPAddressWithPort
	}
	server.NeighborsLock.RUnlock()

	for addr, ipWithPort := range neighborsTmp {
		if addr == exclude {
			continue
		}

		request := getSomeRequestByAddress(addr, false)
		sendReply(getMessage(data, request, request == nil, ipWithPort, nil))
		sent++
	}

	return sent
}

func pendingOnLoad() {
	PendingRequests = make(map[string]*PendingRequest)
	loadPendingRequests()
}

func loadPendingRequests() {
	// TODO: if pending is pending for too long, remove it from the loop?
	logs.Log.Info("Loading pending requests")

	db.Singleton.Lock()
	defer db.Singleton.Unlock()
	RequestQueuesLock.Lock()
	defer RequestQueuesLock.Unlock()

	total := 0
	added := 0

	db.Singleton.View(func(tx db.Transaction) error {
		return coding.ForPrefixBytes(tx, []byte{db.KEY_PENDING_HASH}, true, func(key, hash []byte) (bool, error) {
			total++

			timestamp, err := coding.GetInt64(tx, db.AsKey(key, db.KEY_PENDING_TIMESTAMP))
			if err != nil {
				logs.Log.Warning("Could not load pending Tx Timestamp")
				return true, nil
			}

			for _, neighbor := range server.Neighbors {
				queue, ok := RequestQueues[neighbor.Addr]
				if !ok {
					q := make(RequestQueue, maxQueueSize)
					queue = &q
					RequestQueues[neighbor.Addr] = queue
				}
				*queue <- &Request{hash, false}
			}
			addPendingRequest(hash, timestamp, "", false)
			added++

			return true, nil
		})
	})

	logs.Log.Info("Pending requests loaded", added, total)
}

func getSomeRequestByAddress(address string, any bool) []byte {
	RequestQueuesLock.RLock()
	requestQueue, requestOk := RequestQueues[address]
	RequestQueuesLock.RUnlock()
	var request []byte
	if requestOk && len(*requestQueue) > 0 {
		request = (<-*requestQueue).Requested
	}
	if request == nil {
		pendingRequest := getOldPending(address)
		if pendingRequest == nil && any {
			pendingRequest = getAnyRandomOldPending(address)
		}
		if pendingRequest != nil {
			request = pendingRequest.Hash
		}
	}
	return request
}

func getSomeRequestByIPAddressWithPort(IPAddressWithPort string, any bool) []byte {
	neighbor := server.GetNeighborByIPAddressWithPort(IPAddressWithPort)

	// On low-end devices, the neighbor might already have gone until the message
	// is dequeued and processed. So, we need to check here if the neighbor is still there.
	if neighbor == nil {
		logs.Log.Debug("Neighbor gone:", IPAddressWithPort)
		return nil
	}

	return getSomeRequestByAddress(neighbor.Addr, any)
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

	var neighborsTmp = make(map[string]string)
	server.NeighborsLock.RLock()
	for _, neighbor := range server.Neighbors {
		neighborsTmp[neighbor.Addr] = neighbor.IPAddressWithPort
	}
	server.NeighborsLock.RUnlock()

	for addr, ipWithPort := range neighborsTmp {
		var request = getSomeRequestByAddress(addr, false)
		if request != nil {
			sendReply(getMessage(nil, request, false, ipWithPort, nil))
		} else if shouldRequestTip {
			lastTip = time.Now()
			sendReply(getMessage(nil, nil, true, ipWithPort, nil))
		}
	}
}

func requestIfMissing(hash []byte, IPAddressWithPort string) (has bool, err error) {
	has = true
	if bytes.Equal(hash, tipFastTX.Hash) {
		return has, nil
	}
	key := db.GetByteKey(hash, db.KEY_HASH)
	if !db.Singleton.HasKey(key) && !db.Singleton.HasKey(db.AsKey(key, db.KEY_PENDING_TIMESTAMP)) {
		pending := addPendingRequest(hash, 0, IPAddressWithPort, true)
		if pending != nil {
			neighbor := server.GetNeighborByIPAddressWithPort(IPAddressWithPort)
			if neighbor != nil {
				RequestQueuesLock.Lock()
				queue, ok := RequestQueues[neighbor.Addr]
				if !ok {
					q := make(RequestQueue, maxQueueSize)
					queue = &q
					RequestQueues[neighbor.Addr] = queue
				}
				RequestQueuesLock.Unlock()
				*queue <- &Request{Requested: hash, Tip: false}
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
	hash := *msg.Requested
	data := append((*msg.Bytes)[:1604], hash[:46]...)

	// Check that the neighbor is connected as is sending transactions
	// Probably a good neighbor with transactions. Set this request as sent.
	LastIncomingTimeLock.RLock()
	lastRequestTime, ok := LastIncomingTime[msg.IPAddressWithPort]
	LastIncomingTimeLock.RUnlock()
	if ok && time.Now().Sub(lastRequestTime) < reRequestInterval {
		coding.IncrementInt64By(db.Singleton, db.GetByteKey(hash, db.KEY_PENDING_REQUESTS), 1, false)
	}

	srv.Outgoing <- &server.Message{IPAddressWithPort: msg.IPAddressWithPort, Msg: data}
	outgoing++
}

func getMessage(resp []byte, req []byte, tip bool, IPAddressWithPort string, tx db.Transaction) *Message {
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
	/*/ Otherwise, latest (youngest) TX
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
	/**/
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
			addr := IPAddressWithPort
			neighbor := server.GetNeighborByIPAddressWithPort(IPAddressWithPort)
			if neighbor != nil {
				addr = neighbor.Addr
			}
			pendingRequest := getOldPending(addr)
			if pendingRequest != nil {
				req = pendingRequest.Hash
			}
			if req == nil {
				pendingRequest = getAnyRandomOldPending(addr)
				if pendingRequest != nil {
					req = pendingRequest.Hash
				}
			}
		}
	}
	if req == nil {
		req = hash
	}
	if req == nil {
		req = make([]byte, 46)
	}
	return &Message{Bytes: &resp, Requested: &req, IPAddressWithPort: IPAddressWithPort}
}

func addPendingRequest(hash []byte, timestamp int64, IPAddressWithPort string, save bool) *PendingRequest {

	key := string(hash)

	PendingRequestsLock.RLock()
	pendingRequest, ok := PendingRequests[key]
	PendingRequestsLock.RUnlock()

	if ok {
		return pendingRequest
	}

	if timestamp == 0 {
		timestamp = time.Now().Add(-reRequestInterval).Unix()
	}

	if save {
		key := db.GetByteKey(hash, db.KEY_PENDING_HASH)
		coding.PutBytes(db.Singleton, key, hash)
		coding.PutInt64(db.Singleton, db.AsKey(key, db.KEY_PENDING_TIMESTAMP), timestamp)
	}

	addr := IPAddressWithPort
	neighbor := server.GetNeighborByIPAddressWithPort(IPAddressWithPort)
	if neighbor != nil {
		addr = neighbor.Addr
	}

	pendingRequest = &PendingRequest{Hash: hash, Timestamp: int(timestamp), LastTried: time.Now().Add(-reRequestInterval), LastNeighborAddr: addr}

	PendingRequestsLock.Lock()
	PendingRequests[key] = pendingRequest
	PendingRequestsLock.Unlock()

	return pendingRequest
}

func removePendingRequest(hash []byte) bool {

	key := string(hash)
	PendingRequestsLock.RLock()
	_, ok := PendingRequests[key]
	PendingRequestsLock.RUnlock()

	if ok {
		PendingRequestsLock.Lock()
		delete(PendingRequests, key)
		PendingRequestsLock.Unlock()
		key := db.GetByteKey(hash, db.KEY_PENDING_HASH)
		db.Singleton.Remove(key)
		db.Singleton.Remove(db.AsKey(key, db.KEY_PENDING_TIMESTAMP))
	}
	return ok
}

func getOldPending(excludeAddress string) *PendingRequest {
	max := 1000
	if lowEndDevice {
		max = 200
	}

	PendingRequestsLock.RLock()
	defer PendingRequestsLock.RUnlock()

	length := len(PendingRequests)
	if length < max {
		max = length
	}

	for i := 0; i < max; i++ {
		k := randmap.FastKey(PendingRequests)
		v := PendingRequests[k.(string)]
		now := time.Now()
		if now.Sub(v.LastTried) > reRequestInterval {
			v.LastTried = now
			v.LastNeighborAddr = excludeAddress
			return v
		}
	}

	return nil
}

func getAnyRandomOldPending(excludeAddress string) *PendingRequest {

	max := 10000
	if lowEndDevice {
		max = 300
	}

	PendingRequestsLock.RLock()
	defer PendingRequestsLock.RUnlock()

	length := len(PendingRequests)
	if length < max {
		max = length
	}

	if max > 0 {
		start := utils.Random(0, max)

		for i := start; i < max; i++ {
			k := randmap.FastKey(PendingRequests)
			v := PendingRequests[k.(string)]
			if v.LastNeighborAddr != excludeAddress {
				v.LastTried = time.Now()
				v.LastNeighborAddr = excludeAddress
				return v
			}
		}
	}

	return nil
}

/**
After a certain amount of requests of specific hash from the neighbors,
if the TX is not received, the requests is deleted. This is probably a fake
or invalid spam referenced by trunk/branch of a transaction. Just ignore.
*/
func cleanupStalledRequests() {

	var keysToRemove [][]byte
	var requestsToRemove []string

	db.Singleton.View(func(tx db.Transaction) error {
		return coding.ForPrefixInt(tx, []byte{db.KEY_PENDING_REQUESTS}, true, func(key []byte, times int) (bool, error) {
			if times > maxTimesRequest {
				keysToRemove = append(keysToRemove, db.AsKey(key, db.KEY_PENDING_REQUESTS))
			}
			return true, nil
		})
	})

	for _, key := range keysToRemove {
		var hash []byte
		err := db.Singleton.Update(func(tx db.Transaction) error {
			if err := tx.Remove(key); err != nil {
				return err
			}
			k := db.AsKey(key, db.KEY_PENDING_HASH)

			err := error(nil)
			hash, err = tx.GetBytes(k)
			if err != nil {
				return nil
			}

			err = tx.Remove(db.AsKey(key, db.KEY_PENDING_HASH))
			if err != nil {
				return err
			}
			err = tx.Remove(db.AsKey(key, db.KEY_PENDING_TIMESTAMP))
			if err != nil {
				return err
			}
			err = tx.Remove(db.AsKey(key, db.KEY_PENDING_CONFIRMED))
			if err != nil {
				return err
			}

			return nil
		})
		if err == nil && hash != nil {
			requestsToRemove = append(requestsToRemove, string(hash))
		}
	}

	PendingRequestsLock.Lock()
	for _, req := range requestsToRemove {
		delete(PendingRequests, req)
	}
	PendingRequestsLock.Unlock()
}
