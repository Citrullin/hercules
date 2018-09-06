package tangle

import (
	"bytes"
	"sync/atomic"
	"time"

	"../convert"
	"../crypt"
	"../db"
	"../db/coding"
	"../logs"
	"../server"
	"../snapshot"
	"../transaction"
	"../utils"
)

const P_TIP_REPLY = 25
const P_BROADCAST = 10

func incomingRunner() {
	for raw := range srv.Incoming {
		// Hard limit for low-end devices. Prevent flooding, discard incoming while the queue is full.
		if lowEndDevice && len(srv.Incoming) > maxIncoming*2 {
			continue
		}

		data := raw.Msg[:1604]
		req := make([]byte, 49)
		copy(req, raw.Msg[1604:1650])

		incoming++

		db.Singleton.Lock()
		db.Singleton.Unlock()

		var hash []byte

		fingerprint := db.GetByteKey(data, db.KEY_FINGERPRINT)
		if !hasFingerprint(fingerprint) {
			// Message was not received in the last time
			trits := convert.BytesToTrits(data)[:8019]
			var tx = transaction.TritsToTX(&trits, data)
			hash = tx.Hash

			if !bytes.Equal(data, tipBytes) {
				// Tx was not a tip
				if !crypt.IsValidPoW(tx.Hash, MWM) {
					// POW invalid => Track invalid messages from neighbor
					server.NeighborTrackingQueue <- &server.NeighborTrackingMessage{IPAddressWithPort: raw.IPAddressWithPort, Invalid: 1}
				} else {
					// POW valid => Process the message
					err := processIncomingTX(IncomingTX{TX: tx, IPAddressWithPort: raw.IPAddressWithPort, Bytes: &data})
					if err == nil {
						incomingProcessed++
						addFingerprint(fingerprint)
						LastIncomingTimeLock.Lock()
						LastIncomingTime[raw.IPAddressWithPort] = time.Now()
						LastIncomingTimeLock.Unlock()
					}
				}
			}
		}

		// Pause for a while without responding to prevent flooding
		if len(srv.Incoming) > maxIncoming {
			continue
		}

		var reply []byte = nil
		var isLookingForTX = !bytes.Equal(req, tipBytes[:49]) && (hash == nil || !bytes.Equal(hash, req))

		if isLookingForTX {
			reply, _ = db.Singleton.GetBytes(db.GetByteKey(req, db.KEY_BYTES))
		} else if utils.Random(0, 100) < P_TIP_REPLY {
			// If this is a tip request, drop randomly
			continue
		}

		request := getSomeRequestByIPAddressWithPort(raw.IPAddressWithPort, false)
		if isLookingForTX && request == nil && reply == nil {
			// If the peer wants a specific TX and we do not have it and we have nothing to ask for,
			// then do not reply. If we do not have it, but have something to ask, then ask.
			continue
		}

		sendReply(getMessage(reply, request, request == nil, raw.IPAddressWithPort, nil))
	}
}

func processIncomingTX(incoming IncomingTX) error {
	t := incoming.TX
	var pendingMilestone *PendingMilestone
	err := db.Singleton.Update(func(tx db.Transaction) (e error) {
		// TODO: catch error defer here

		key := db.GetByteKey(t.Hash, db.KEY_HASH)
		removePendingRequest(t.Hash)

		snapTime := snapshot.GetSnapshotTimestamp(tx)

		removeTx := func() {
			//logs.Log.Debugf("Skipping this TX: %v", convert.BytesToTrytes(tx.Hash)[:81])
			tx.Remove(db.AsKey(key, db.KEY_PENDING_CONFIRMED))
			tx.Remove(db.AsKey(key, db.KEY_EVENT_CONFIRMATION_PENDING))
			tx.Remove(db.AsKey(key, db.KEY_EVENT_MILESTONE_PAIR_PENDING)) // TODO
			err := coding.PutInt64(tx, db.AsKey(key, db.KEY_EDGE), snapTime)
			_checkIncomingError(t, err)
			parentKey, err := tx.GetBytes(db.AsKey(key, db.KEY_EVENT_MILESTONE_PAIR_PENDING))
			if err == nil {
				err = tx.Remove(db.AsKey(parentKey, db.KEY_EVENT_MILESTONE_PENDING))
				_checkIncomingError(t, err)
			}
		}

		futureTime := int(time.Now().Add(2 * time.Hour).Unix())
		maybeMilestonePair := isMaybeMilestone(t) || isMaybeMilestonePair(t)
		isOutsideOfTimeframe := !maybeMilestonePair && (t.Timestamp > futureTime || snapTime >= int64(t.Timestamp))
		if isOutsideOfTimeframe && !tx.HasKey(db.GetByteKey(t.Bundle, db.KEY_PENDING_BUNDLE)) {
			removeTx()
			return nil
		}

		if tx.HasKey(db.AsKey(key, db.KEY_SNAPSHOTTED)) {
			_, err := requestIfMissing(t.TrunkTransaction, incoming.IPAddressWithPort)
			_checkIncomingError(t, err)
			_, err = requestIfMissing(t.BranchTransaction, incoming.IPAddressWithPort)
			_checkIncomingError(t, err)
			removeTx()
			return nil
		}

		if !tx.HasKey(key) {
			// Tx is not in the database yet

			err := SaveTX(t, incoming.Bytes, tx)
			_checkIncomingError(t, err)
			if isMaybeMilestone(t) {
				trunkBytesKey := db.GetByteKey(t.TrunkTransaction, db.KEY_BYTES)
				err := tx.PutBytes(db.AsKey(key, db.KEY_EVENT_MILESTONE_PENDING), trunkBytesKey)
				_checkIncomingError(t, err)

				pendingMilestone = &PendingMilestone{key, trunkBytesKey}
			}
			_, err = requestIfMissing(t.TrunkTransaction, incoming.IPAddressWithPort)
			_checkIncomingError(t, err)

			_, err = requestIfMissing(t.BranchTransaction, incoming.IPAddressWithPort)
			_checkIncomingError(t, err)

			// EVENTS:

			pendingConfirmationKey := db.AsKey(key, db.KEY_PENDING_CONFIRMED)
			if tx.HasKey(pendingConfirmationKey) {
				err = tx.Remove(pendingConfirmationKey)
				_checkIncomingError(t, err)

				err = addPendingConfirmation(db.AsKey(key, db.KEY_EVENT_CONFIRMATION_PENDING), int64(t.Timestamp), tx)
				_checkIncomingError(t, err)
			}

			parentKey, err := tx.GetBytes(db.AsKey(key, db.KEY_EVENT_MILESTONE_PAIR_PENDING))
			if err == nil {
				pendingMilestone = &PendingMilestone{parentKey, db.AsKey(key, db.KEY_BYTES)}
			}

			// Re-broadcast new TX. Not always.
			// Here, it is actually possible to favor nearer neighbours!
			if (!lowEndDevice || len(srv.Incoming) < maxIncoming) && utils.Random(0, 100) < P_BROADCAST {
				Broadcast(t.Bytes, incoming.IPAddressWithPort)
			}

			server.NeighborTrackingQueue <- &server.NeighborTrackingMessage{IPAddressWithPort: incoming.IPAddressWithPort, New: 1}
			saved++
			atomic.AddInt64(&totalTransactions, 1)
		} else {
			discarded++
		}
		return nil
	})

	if err == nil {
		if pendingMilestone != nil {
			addPendingMilestoneToQueue(pendingMilestone)
		}
	} else {
		addPendingRequest(t.Hash, 0, incoming.IPAddressWithPort, true)

		atomic.AddInt64(&totalTransactions, -1)
		server.NeighborTrackingQueue <- &server.NeighborTrackingMessage{IPAddressWithPort: incoming.IPAddressWithPort, New: -1}
	}
	return err
}

func _checkIncomingError(tx *transaction.FastTX, err error) {
	if err != nil {
		logs.Log.Errorf("Failed processing TX %v", convert.BytesToTrytes(tx.Hash)[:81], err)
		panic(err)
	}
}

func cleanupRequestQueues() {
	RequestQueuesLock.RLock()
	var toRemove []string
	for address := range RequestQueues {
		if server.GetNeighborByAddress(address) == nil {
			toRemove = append(toRemove, address)
		}
	}
	RequestQueuesLock.RUnlock()

	RequestQueuesLock.Lock()
	LastIncomingTimeLock.Lock()
	if toRemove != nil && len(toRemove) > 0 {
		for _, address := range toRemove {
			logs.Log.Debug("Removing gone neighbor queue for:", address)
			delete(RequestQueues, address)
			delete(LastIncomingTime, address)
		}
	}
	RequestQueuesLock.Unlock()
	LastIncomingTimeLock.Unlock()
}
