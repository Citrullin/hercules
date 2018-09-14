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
	srv.IncomingWaitGroup.Add(1)
	defer srv.IncomingWaitGroup.Done()

	for raw := range srv.Incoming {
		// Hard limit for low-end devices. Prevent flooding, discard incoming while the queue is full.
		if lowEndDevice && len(srv.Incoming) > maxIncoming*2 {
			continue
		}

		ipAddressWithPort := (*raw.Addr).String() // Format <ip>:<port>

		ipIsBlocked := server.IsIPBlocked(ipAddressWithPort)
		if ipIsBlocked {
			// Skip message
			continue
		}

		server.NeighborsLock.RLock()
		neighborExists, neighbor := server.CheckNeighbourExistsByIPAddressWithPort(ipAddressWithPort, false)
		if !neighborExists {
			// Check all known addresses => slower
			neighborExists, neighbor = server.CheckNeighbourExistsByIPAddressWithPort(ipAddressWithPort, true)

			server.NeighborsLock.RUnlock()

			if neighborExists {
				// If the neighbor was found now, the preferred IP is wrong => Update it!
				neighbor.UpdateIPAddressWithPort(ipAddressWithPort)
			} else {
				logs.Log.Infof("Blocked unknown neighbor (%v)", ipAddressWithPort)
				server.BlockIP(ipAddressWithPort)
				continue
			}
		} else {
			server.NeighborsLock.RUnlock()
		}

		atomic.AddUint64(&server.IncTxPerSec, 1)
		neighbor.TrackIncoming(1)

		data := (*raw.Data)[:DATA_SIZE]
		reqHash := make([]byte, HASH_SIZE)
		copy(reqHash, (*raw.Data)[DATA_SIZE:PACKET_SIZE])

		var recHash []byte

		//
		// Process incoming data
		//
		fingerprintHash := ns.HashKey(data, ns.NamespaceFingerprint)
		fingerprint := getFingerprint(fingerprintHash)
		if fingerprint == nil {
			// Message was not received in the last time

			trits := convert.BytesToTrits(data)[:TX_TRITS_LENGTH]
			tx := transaction.TritsToTX(&trits, data)
			recHash = tx.Hash

			// Check again because another thread could have received
			// the same message from another neighbor in the meantime
			fpExists := addFingerprint(fingerprintHash, recHash)

			if !fpExists {
				// Message was not received in the mean time
				atomic.AddUint64(&server.NewTxPerSec, 1)

				if !bytes.Equal(data, tipBytes) {
					// Tx was not a tip
					if crypt.IsValidPoW(tx.Hash, MWM) {
						// POW valid => Process the message
						err := processIncomingTX(tx, neighbor)
						if err != nil {
							if err == db.ErrTransactionConflict {
								removeFingerprint(fingerprintHash)
								srv.Incoming <- raw
								continue
							}
							logs.Log.Errorf("Processing incoming message failed! Err: %v", err)
						} else {
							atomic.AddUint64(&server.ValidTxPerSec, 1)
							neighbor.LastIncomingTime = time.Now()
						}
					} else {
						// POW invalid => Track invalid messages from neighbor
						neighbor.TrackInvalid(1)
					}
				} else {
					atomic.AddUint64(&server.TipReqPerSec, 1)
				}
			} else {
				atomic.AddUint64(&server.KnownTxPerSec, 1)
			}
		} else {
			atomic.AddUint64(&server.KnownTxPerSec, 1)
			recHash = fingerprint.ReceiveHash
		}

		//
		// Reply to incoming request
		//

		// Pause for a while without responding to prevent flooding
		if len(srv.Incoming) > maxIncoming {
			continue
		}

		var reply []byte
		var isLookingForTX = !bytes.Equal(reqHash, tipBytes[:HASH_SIZE]) && (!bytes.Equal(recHash, reqHash))

		if isLookingForTX {
			reply, _ = db.Singleton.GetBytes(ns.HashKey(reqHash, ns.NamespaceBytes))
		} else if utils.Random(0, 100) < P_TIP_REPLY {
			// If this is a tip request, drop randomly
			continue
		}

		request := getSomeRequestByNeighbor(neighbor, false)
		if isLookingForTX && request == nil && reply == nil {
			// If the peer wants a specific TX and we do not have it and we have nothing to ask for,
			// then do not reply. If we do not have it, but have something to ask, then ask.
			continue
		}

		sendReply(getMessage(reply, request, request == nil, neighbor, nil))
	}
}

func processIncomingTX(t *transaction.FastTX, neighbor *server.Neighbor) error {
	var pendingMilestone *PendingMilestone

	snapTime := snapshot.GetSnapshotTimestamp(nil)
	key := ns.HashKey(t.Hash, ns.NamespaceHash)
	futureTime := int(time.Now().Add(2 * time.Hour).Unix())
	maybeMilestonePair := isMaybeMilestone(t) || isMaybeMilestonePair(t)
	isOutsideOfTimeframe := !maybeMilestonePair && (t.Timestamp > futureTime || snapTime >= int64(t.Timestamp))

	err := db.Singleton.Update(func(tx db.Transaction) (e error) {
		// TODO: catch error defer here

		removePendingRequest(t.Hash, tx)

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

		if isOutsideOfTimeframe && !tx.HasKey(ns.HashKey(t.Bundle, ns.NamespacePendingBundle)) {
			removeTx()
			return nil
		}

		if tx.HasKey(ns.Key(key, ns.NamespaceSnapshotted)) {
			_, err := requestIfMissing(t.TrunkTransaction, neighbor)
			_checkIncomingError(t, err)
			_, err = requestIfMissing(t.BranchTransaction, neighbor)
			_checkIncomingError(t, err)
			removeTx()
			return nil
		}

		if !tx.HasKey(key) {
			// Tx is not in the database yet

			err := SaveTX(t, &t.Bytes, tx)
			_checkIncomingError(t, err)
			if isMaybeMilestone(t) {
				trunkBytesKey := ns.HashKey(t.TrunkTransaction, ns.NamespaceBytes)
				err := tx.PutBytes(ns.Key(key, ns.NamespaceEventMilestonePending), trunkBytesKey)
				_checkIncomingError(t, err)

				pendingMilestone = &PendingMilestone{key, trunkBytesKey}
			}
			_, err = requestIfMissing(t.TrunkTransaction, neighbor)
			_checkIncomingError(t, err)
			_, err = requestIfMissing(t.BranchTransaction, neighbor)
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
				Broadcast(t.Bytes, neighbor)
			}

			neighbor.TrackNew(1)
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
		addPendingRequest(t.Hash, 0, neighbor, true)

		atomic.AddInt64(&totalTransactions, -1)
		neighbor.TrackNew(-1)
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
