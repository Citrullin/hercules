package tangle

import (
	"bytes"
	"sync"
	"time"

	"../db"
	"../db/coding"
	"../db/ns"
	"../logs"
	"../server"
	"../utils"

	"github.com/lukechampine/randmap" // github.com/lukechampine/randmap/safe is safer, but for now we use the faster one
)

const (
	tipRequestInterval = time.Duration(200) * time.Millisecond
	reRequestInterval  = time.Duration(10) * time.Second
	maxIncoming        = 100
	maxTimesRequest    = 100
)

var (
	lastTip                  = time.Now()
	PendingRequests          map[string]*PendingRequest
	PendingRequestsLock      = &sync.RWMutex{}
	outgoingRunnerTicker     *time.Ticker
	outgoingRunnerWaitGroup  = &sync.WaitGroup{}
	outgoingRunnerTickerQuit = make(chan struct{})
)

type PendingRequest struct {
	Hash      []byte
	Timestamp int
	LastTried time.Time
	Neighbor  *server.Neighbor
}

func Broadcast(data []byte, excludeNeighbor *server.Neighbor) int {
	sent := 0

	server.NeighborsLock.RLock()
	for _, neighbor := range server.Neighbors {
		if neighbor == excludeNeighbor {
			continue
		}

		request := getSomeRequestByNeighbor(neighbor, false)
		sendReply(getMessage(data, request, request == nil, neighbor, nil))
		sent++
	}
	server.NeighborsLock.RUnlock()

	return sent
}

func pendingOnLoad() {
	PendingRequests = make(map[string]*PendingRequest)
	loadPendingRequests()
}

func loadPendingRequests() {
	// TODO: if pending is pending for too long, remove it from the loop?
	logs.Log.Info("Loading pending requests")

	RequestQueuesLock.Lock()
	defer RequestQueuesLock.Unlock()

	total := 0
	added := 0

	db.Singleton.View(func(tx db.Transaction) error {
		return ns.ForNamespace(tx, ns.NamespacePendingHash, true, func(key, hash []byte) (bool, error) {
			total++

			timestamp, err := coding.GetInt64(tx, ns.Key(key, ns.NamespacePendingTimestamp))
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
			addPendingRequest(hash, timestamp, nil, false)
			added++

			return true, nil
		})
	})

	logs.Log.Info("Pending requests loaded", added, total)
}

func getSomeRequestByNeighbor(neighbor *server.Neighbor, any bool) []byte {
	if neighbor == nil {
		return nil
	}

	RequestQueuesLock.RLock()
	requestQueue, requestOk := RequestQueues[neighbor.Addr]
	RequestQueuesLock.RUnlock()

	var request []byte
	if requestOk && len(*requestQueue) > 0 {
		request = (<-*requestQueue).Requested
	}

	if request == nil {
		pendingRequest := getOldPending(neighbor)
		if pendingRequest == nil && any {
			pendingRequest = getAnyRandomOldPending(neighbor)
		}
		if pendingRequest != nil {
			request = pendingRequest.Hash
		}
	}
	return request
}

func outgoingRunner() {
	outgoingRunnerWaitGroup.Add(1)
	defer outgoingRunnerWaitGroup.Done()

	executeOutgoingRunner()

	outgoingInterval := 2 * time.Millisecond
	if lowEndDevice {
		outgoingInterval *= 5
	}

	outgoingRunnerTicker = time.NewTicker(outgoingInterval)
	for {
		select {
		case <-outgoingRunnerTickerQuit:
			return

		case <-outgoingRunnerTicker.C:
			if ended {
				break
			}
			executeOutgoingRunner()
		}
	}
}

func executeOutgoingRunner() {
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
		request := getSomeRequestByNeighbor(neighbor, false)
		if request != nil {
			sendReply(getMessage(nil, request, false, neighbor, nil))
		} else if shouldRequestTip {
			lastTip = time.Now()
			sendReply(getMessage(nil, nil, true, neighbor, nil))
		}
	}
	server.NeighborsLock.RUnlock()
}

func requestIfMissing(hash []byte, neighbor *server.Neighbor) (has bool, err error) {
	has = true
	if bytes.Equal(hash, tipFastTX.Hash) {
		return has, nil
	}
	key := ns.HashKey(hash, ns.NamespaceHash)
	if !db.Singleton.HasKey(key) && !db.Singleton.HasKey(ns.Key(key, ns.NamespacePendingTimestamp)) {
		pending := addPendingRequest(hash, 0, neighbor, true)
		if pending != nil {
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

	if msg.Neighbor != nil {
		// Check that the neighbor is connected as is sending transactions
		// Probably a good neighbor with transactions. Set this request as sent.

		if time.Now().Sub(msg.Neighbor.LastIncomingTime) < reRequestInterval {
			coding.IncrementInt64By(db.Singleton, ns.HashKey(hash, ns.NamespacePendingRequests), 1, false)
		}
	}

	srv.Outgoing <- &server.Message{Neighbor: msg.Neighbor, Msg: data}
	outgoing++
}

func getMessage(resp []byte, req []byte, tip bool, neighbor *server.Neighbor, tx db.Transaction) *Message {
	var hash []byte
	if resp == nil {
		hash, resp = getRandomTip(tx)
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
		key, _, _ := db.GetLatestKey(ns.NamespaceTimestamp, false, txn)
		if key != nil {
			key = ns.Key(key, ns.NamespaceBytes)
			resp, _ = db.GetBytes(key, txn)
			if req == nil {
				key = ns.Key(key, ns.NamespaceHash)
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
			pendingRequest := getOldPending(neighbor)
			if pendingRequest != nil {
				req = pendingRequest.Hash
			}
			if req == nil {
				pendingRequest = getAnyRandomOldPending(neighbor)
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
	return &Message{Bytes: &resp, Requested: &req, Neighbor: neighbor}
}

func addPendingRequest(hash []byte, timestamp int64, neighbor *server.Neighbor, save bool) *PendingRequest {

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
		key := ns.HashKey(hash, ns.NamespacePendingHash)
		tx := db.Singleton.NewTransaction(true)
		tx.PutBytes(key, hash)
		coding.PutInt64(tx, ns.Key(key, ns.NamespacePendingTimestamp), timestamp)
		tx.Commit()
	}

	pendingRequest = &PendingRequest{Hash: hash, Timestamp: int(timestamp), LastTried: time.Now().Add(-reRequestInterval), Neighbor: neighbor}
	if len(pendingRequest.Hash) == 0 {
		logs.Log.Panic("addPendingRequest 0 hash")
	}
	PendingRequestsLock.Lock()
	PendingRequests[key] = pendingRequest
	PendingRequestsLock.Unlock()

	return pendingRequest
}

func removePendingRequest(hash []byte, tx db.Transaction) bool {

	key := string(hash)
	PendingRequestsLock.RLock()
	_, ok := PendingRequests[key]
	PendingRequestsLock.RUnlock()

	if ok {
		PendingRequestsLock.Lock()
		delete(PendingRequests, key)
		PendingRequestsLock.Unlock()

		key := ns.HashKey(hash, ns.NamespacePendingHash)
		tx.Remove(key)
		tx.Remove(ns.Key(key, ns.NamespacePendingTimestamp))
	}
	return ok
}

func getOldPending(neighbor *server.Neighbor) *PendingRequest {

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

	now := time.Now()
	for i := 0; i < max; i++ {
		k := randmap.FastKey(PendingRequests).(string)
		v := PendingRequests[k]
		if now.Sub(v.LastTried) > reRequestInterval {
			v.LastTried = now
			v.Neighbor = neighbor
			return v
		}
	}

	return nil
}

func getAnyRandomOldPending(excludeNeighbor *server.Neighbor) *PendingRequest {

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
			k := randmap.FastKey(PendingRequests).(string)
			v := PendingRequests[k]
			if v.Neighbor != excludeNeighbor {
				v.LastTried = time.Now()
				v.Neighbor = excludeNeighbor
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

	db.Singleton.View(func(tx db.Transaction) error {
		return coding.ForPrefixInt(tx, ns.Prefix(ns.NamespacePendingRequests), true, func(key []byte, times int) (bool, error) {
			if times > maxTimesRequest {
				keysToRemove = append(keysToRemove, ns.Key(key, ns.NamespacePendingRequests))
			}
			return true, nil
		})
	})

	var requestsToRemove []string

	db.Singleton.Update(func(tx db.Transaction) (err error) {
		for _, key := range keysToRemove {
			var hash []byte

			err = tx.Remove(key)
			if err != nil {
				return err
			}

			keyPendingHash := ns.Key(key, ns.NamespacePendingHash)
			hash, err = tx.GetBytes(keyPendingHash)
			if err != nil {
				return nil
			}

			err = tx.Remove(keyPendingHash)
			if err != nil {
				return err
			}
			err = tx.Remove(ns.Key(key, ns.NamespacePendingTimestamp))
			if err != nil {
				return err
			}
			err = tx.Remove(ns.Key(key, ns.NamespacePendingConfirmed))
			if err != nil {
				return err
			}

			if hash != nil {
				requestsToRemove = append(requestsToRemove, string(hash))
			}
		}
		return nil
	})

	PendingRequestsLock.Lock()
	for _, req := range requestsToRemove {
		delete(PendingRequests, req)
	}
	PendingRequestsLock.Unlock()
}
