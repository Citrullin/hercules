package tangle

import (
	"bytes"
	"sync/atomic"
	"time"

	"gitlab.com/semkodev/hercules/convert"
	"gitlab.com/semkodev/hercules/crypt"
	"gitlab.com/semkodev/hercules/db"
	"gitlab.com/semkodev/hercules/db/coding"
	"gitlab.com/semkodev/hercules/db/ns"
	"gitlab.com/semkodev/hercules/logs"
	"gitlab.com/semkodev/hercules/server"
	"gitlab.com/semkodev/hercules/snapshot"
	"gitlab.com/semkodev/hercules/transaction"
	"gitlab.com/semkodev/hercules/utils"
)

const (
	P_TIP_REPLY = 25
	P_BROADCAST = 10
)

func incomingRunner() {
	srv.IncomingWaitGroup.Add(1)
	defer srv.IncomingWaitGroup.Done()

	for {
		select {
		case <-srv.IncomingQueueQuit:
			return

		case raw := <-srv.Incoming:
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
			fingerprintHash, err := fingerPrintCache.Get(data)
			if err != nil {
				// Message was not received in the last time

				trits := convert.BytesToTrits(data)[:TX_TRITS_LENGTH]
				tx := transaction.TritsToTX(&trits, data)
				recHash = tx.Hash

				// Check again because another thread could have received
				// the same message from another neighbor in the meantime
				fingerprintHash, err = fingerPrintCache.Get(data)
				if err != nil {
					fingerPrintCache.Set(data, recHash, fingerPrintTTL)

					// Message was not received in the mean time
					atomic.AddUint64(&server.NewTxPerSec, 1)

					if !bytes.Equal(data, tipBytes) {
						// Tx was not a tip
						if crypt.IsValidPoW(tx.Hash, MWM) {
							// POW valid => Process the message
							err = processIncomingTX(tx, neighbor)
							if err != nil {
								if err == db.ErrTransactionConflict {
									fingerPrintCache.Del(data)
									srv.Incoming <- raw // Process the message again
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
				recHash = fingerprintHash
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
}

func processIncomingTX(tx *transaction.FastTX, neighbor *server.Neighbor) error {
	var pendingMilestone *PendingMilestone

	snapTime := snapshot.GetSnapshotTimestamp(nil)
	key := ns.HashKey(tx.Hash, ns.NamespaceHash)
	futureTime := int(time.Now().Add(2 * time.Hour).Unix())
	maybeMilestonePair := isMaybeMilestone(tx) || isMaybeMilestonePair(tx)
	isOutsideOfTimeframe := !maybeMilestonePair && (tx.Timestamp > futureTime || snapTime >= int64(tx.Timestamp))

	err := db.Singleton.Update(func(dbTx db.Transaction) (e error) {
		// TODO: catch error defer here

		removePendingRequest(tx.Hash, dbTx)

		removeTx := func() {
			//logs.Log.Debugf("Skipping this TX: %v", convert.BytesToTrytes(tx.Hash)[:81])
			dbTx.Remove(ns.Key(key, ns.NamespacePendingConfirmed))
			dbTx.Remove(ns.Key(key, ns.NamespaceEventConfirmationPending))
			err := coding.PutInt64(dbTx, ns.Key(key, ns.NamespaceEdge), snapTime)
			_checkIncomingError(tx, err)
			parentKey, err := dbTx.GetBytes(ns.Key(key, ns.NamespaceEventMilestonePairPending))
			if err == nil {
				err = dbTx.Remove(ns.Key(parentKey, ns.NamespaceEventMilestonePending))
				_checkIncomingError(tx, err)
			}
			dbTx.Remove(ns.Key(key, ns.NamespaceEventMilestonePairPending))
		}

		if isOutsideOfTimeframe && !dbTx.HasKey(ns.HashKey(tx.Bundle, ns.NamespacePendingBundle)) {
			removeTx()
			return nil
		}

		if dbTx.HasKey(ns.Key(key, ns.NamespaceSnapshotted)) {
			_, err := requestIfMissing(tx.TrunkTransaction, neighbor)
			_checkIncomingError(tx, err)
			_, err = requestIfMissing(tx.BranchTransaction, neighbor)
			_checkIncomingError(tx, err)
			removeTx()
			return nil
		}

		if !dbTx.HasKey(key) {
			// Tx is not in the database yet

			err := SaveTX(tx, &tx.Bytes, dbTx)
			_checkIncomingError(tx, err)
			if isMaybeMilestone(tx) {
				trunkBytesKey := ns.HashKey(tx.TrunkTransaction, ns.NamespaceBytes)
				err := dbTx.PutBytes(ns.Key(key, ns.NamespaceEventMilestonePending), trunkBytesKey)
				_checkIncomingError(tx, err)

				pendingMilestone = &PendingMilestone{key, trunkBytesKey}
			}
			_, err = requestIfMissing(tx.TrunkTransaction, neighbor)
			_checkIncomingError(tx, err)
			_, err = requestIfMissing(tx.BranchTransaction, neighbor)
			_checkIncomingError(tx, err)

			// EVENTS:

			pendingConfirmationKey := ns.Key(key, ns.NamespacePendingConfirmed)
			if dbTx.HasKey(pendingConfirmationKey) {
				err = dbTx.Remove(pendingConfirmationKey)
				_checkIncomingError(tx, err)

				err = addPendingConfirmation(ns.Key(key, ns.NamespaceEventConfirmationPending), int64(tx.Timestamp), dbTx)
				_checkIncomingError(tx, err)
			}

			parentKey, err := dbTx.GetBytes(ns.Key(key, ns.NamespaceEventMilestonePairPending))
			if err == nil {
				pendingMilestone = &PendingMilestone{parentKey, ns.Key(key, ns.NamespaceBytes)}
			}

			// Re-broadcast new TX. Not always.
			// Here, it is actually possible to favor nearer neighbours!
			if (!lowEndDevice || len(srv.Incoming) < maxIncoming) && utils.Random(0, 100) < P_BROADCAST {
				Broadcast(tx.Bytes, neighbor)
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
		addPendingRequest(tx.Hash, 0, neighbor, true)

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
