package tangle

import (
	"bytes"
	"sync/atomic"
	"time"

	"../convert"
	"../crypt"
	"../db"
	"../db/coding"
	"../db/ns"
	"../logs"
	"../server"
	"../snapshot"
	"../transaction"
	"../utils"
)

const (
	P_TIP_REPLY = 25
	P_BROADCAST = 10
)

func incomingRunner() {
	if srv == nil {
		logs.Log.Info("empty")
	}

	for raw := range srv.Incoming {
		// Hard limit for low-end devices. Prevent flooding, discard incoming while the queue is full.
		if lowEndDevice && len(srv.Incoming) > maxIncoming*2 {
			continue
		}

		data := raw.Msg[:DATA_SIZE]
		req := make([]byte, HASH_SIZE)
		copy(req, raw.Msg[DATA_SIZE:PACKET_SIZE])

		incoming++

		db.Singleton.Lock()
		db.Singleton.Unlock()

		var hash []byte

		fingerprint := ns.HashKey(data, ns.NamespaceFingerprint)
		if !hasFingerprint(fingerprint) {
			// Message was not received in the last time
			trits := convert.BytesToTrits(data)[:TX_TRITS_LENGTH]
			var tx = transaction.TritsToTX(&trits, data)
			hash = tx.Hash

			if !bytes.Equal(data, tipBytes) {
				// Tx was not a tip
				if !crypt.IsValidPoW(tx.Hash, MWM) {
					// POW invalid => Track invalid messages from neighbor
		            raw.Neighbor.TrackInvalid(1)
				} else {
					// POW valid => Process the message
		            raw.Neighbor.TrackIncoming(1)
					if err == nil {
						incomingProcessed++
						addFingerprint(fingerprint)
						if raw.Neighbor != nil {
							raw.Neighbor.LastIncomingTime = time.Now()
						}
					}
				}
			}
		}

		// Pause for a while without responding to prevent flooding
		if len(srv.Incoming) > maxIncoming {
			continue
		}

		var reply []byte = nil
		var isLookingForTX = !bytes.Equal(req, tipBytes[:HASH_SIZE]) && (hash == nil || !bytes.Equal(hash, req))

		if isLookingForTX {
			reply, _ = db.Singleton.GetBytes(ns.HashKey(req, ns.NamespaceBytes))
		} else if utils.Random(0, 100) < P_TIP_REPLY {
			// If this is a tip request, drop randomly
			continue
		}

		request := getSomeRequestByNeighbor(raw.Neighbor, false)
		if isLookingForTX && request == nil && reply == nil {
			// If the peer wants a specific TX and we do not have it and we have nothing to ask for,
			// then do not reply. If we do not have it, but have something to ask, then ask.
			continue
		}

		sendReply(getMessage(reply, request, request == nil, raw.Neighbor, nil))
	}
}

func processIncomingTX(incoming IncomingTX) error {
	t := incoming.TX
	var pendingMilestone *PendingMilestone

	err := db.Singleton.Update(func(tx db.Transaction) (e error) {
		// TODO: catch error defer here

		key := ns.HashKey(t.Hash, ns.NamespaceHash)
		removePendingRequest(t.Hash)

		snapTime := snapshot.GetSnapshotTimestamp(tx)

		removeTx := func() {
			//logs.Log.Debugf("Skipping this TX: %v", convert.BytesToTrytes(tx.Hash)[:81])
			tx.Remove(ns.Key(key, ns.NamespacePendingConfirmed))
			tx.Remove(ns.Key(key, ns.NamespaceEventConfirmationPending))
			err := coding.PutInt64(tx, ns.Key(key, ns.NamespaceEdge), snapTime)
			_checkIncomingError(t, err)
			parentKey, err := tx.GetBytes(ns.Key(key, ns.NamespaceEventMilestonePairPending))
			if err == nil {
				err = tx.Remove(ns.Key(parentKey, ns.NamespaceEventMilestonePending))
				_checkIncomingError(t, err)
			}
			tx.Remove(ns.Key(key, ns.NamespaceEventMilestonePairPending))
		}

		futureTime := int(time.Now().Add(2 * time.Hour).Unix())
		maybeMilestonePair := isMaybeMilestone(t) || isMaybeMilestonePair(t)
		isOutsideOfTimeframe := !maybeMilestonePair && (t.Timestamp > futureTime || snapTime >= int64(t.Timestamp))
		if isOutsideOfTimeframe && !tx.HasKey(ns.HashKey(t.Bundle, ns.NamespacePendingBundle)) {
			removeTx()
			return nil
		}

		if tx.HasKey(ns.Key(key, ns.NamespaceSnapshotted)) {
			_, err := requestIfMissing(t.TrunkTransaction, incoming.Neighbor)
			_checkIncomingError(t, err)
			_, err = requestIfMissing(t.BranchTransaction, incoming.Neighbor)
			_checkIncomingError(t, err)
			removeTx()
			return nil
		}

		if !tx.HasKey(key) {
			// Tx is not in the database yet

			err := SaveTX(t, incoming.Bytes, tx)
			_checkIncomingError(t, err)
			if isMaybeMilestone(t) {
				trunkBytesKey := ns.HashKey(t.TrunkTransaction, ns.NamespaceBytes)
				err := tx.PutBytes(ns.Key(key, ns.NamespaceEventMilestonePending), trunkBytesKey)
				_checkIncomingError(t, err)

				pendingMilestone = &PendingMilestone{key, trunkBytesKey}
			}
			_, err = requestIfMissing(t.TrunkTransaction, incoming.Neighbor)
			_checkIncomingError(t, err)
			_, err = requestIfMissing(t.BranchTransaction, incoming.Neighbor)
			_checkIncomingError(t, err)

			// EVENTS:

			pendingConfirmationKey := ns.Key(key, ns.NamespacePendingConfirmed)
			if tx.HasKey(pendingConfirmationKey) {
				err = tx.Remove(pendingConfirmationKey)
				_checkIncomingError(t, err)

				err = addPendingConfirmation(ns.Key(key, ns.NamespaceEventConfirmationPending), int64(t.Timestamp), tx)
				_checkIncomingError(t, err)
			}

			parentKey, err := tx.GetBytes(ns.Key(key, ns.NamespaceEventMilestonePairPending))
			if err == nil {
				pendingMilestone = &PendingMilestone{parentKey, ns.Key(key, ns.NamespaceBytes)}
			}

			// Re-broadcast new TX. Not always.
			// Here, it is actually possible to favor nearer neighbours!
			if (!lowEndDevice || len(srv.Incoming) < maxIncoming) && utils.Random(0, 100) < P_BROADCAST {
				Broadcast(t.Bytes, incoming.Neighbor)
			}

			incoming.Neighbor.TrackNew(1)
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
		addPendingRequest(t.Hash, 0, incoming.Neighbor, true)

		atomic.AddInt64(&totalTransactions, -1)
		incoming.Neighbor.TrackNew(-1)
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

	if toRemove != nil && len(toRemove) > 0 {
		RequestQueuesLock.Lock()
		for _, address := range toRemove {
			logs.Log.Debug("Removing gone neighbor queue for:", address)
			delete(RequestQueues, address)
		}
		RequestQueuesLock.Unlock()
	}
}
