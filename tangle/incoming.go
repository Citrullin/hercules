package tangle

import (
	"bytes"
	"sync/atomic"
	"time"

	"../convert"
	"../crypt"
	"../db"
	"../logs"
	"../server"
	"../snapshot"
	"../transaction"
	"../utils"
	"github.com/dgraph-io/badger"
)

const P_TIP_REPLY = 25
const P_BROADCAST = 10

func incomingRunner() {
	for raw := range srv.Incoming {
		data := raw.Msg[:1604]
		req := make([]byte, 49)
		copy(req, raw.Msg[1604:1650])

		if snapshot.InProgress {
			time.Sleep(time.Duration(1) * time.Second)
			continue
		}

		incoming++

		db.Locker.Lock()
		db.Locker.Unlock()

		var hash []byte

		fingerprint := db.GetByteKey(data, db.KEY_FINGERPRINT)
		if !hasFingerprint(fingerprint) {
			trits := convert.BytesToTrits(data)[:8019]
			var tx = transaction.TritsToTX(&trits, data)
			hash = tx.Hash

			if !bytes.Equal(data, tipBytes) {
				if !crypt.IsValidPoW(tx.Hash, MWM) {
					server.NeighborTrackingQueue <- &server.NeighborTrackingMessage{IPAddressWithPort: raw.IPAddressWithPort, Invalid: 1}
				} else {
					err := processIncomingTX(IncomingTX{TX: tx, IPAddressWithPort: raw.IPAddressWithPort, Bytes: &data})
					if err == nil {
						incomingProcessed++
						addFingerprint(fingerprint)
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
			reply, _ = db.GetBytes(db.GetByteKey(req, db.KEY_BYTES), nil)
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
	tx := incoming.TX
	var pendingMilestone *PendingMilestone
	err := db.DB.Update(func(txn *badger.Txn) (e error) {
		// TODO: catch error defer here
		var key = db.GetByteKey(tx.Hash, db.KEY_HASH)
		removePendingRequest(tx.Hash)

		removeTx := func() {
			//logs.Log.Debugf("Skipping this TX: %v", convert.BytesToTrytes(tx.Hash)[:81])
			db.Remove(db.AsKey(key, db.KEY_PENDING_CONFIRMED), txn)
			db.Remove(db.AsKey(key, db.KEY_EVENT_CONFIRMATION_PENDING), txn)
			db.Remove(db.AsKey(key, db.KEY_EVENT_MILESTONE_PAIR_PENDING), txn)
			err := db.Put(db.AsKey(key, db.KEY_EDGE), true, nil, txn)
			_checkIncomingError(tx, err)
			parentKey, err := db.GetBytes(db.AsKey(key, db.KEY_EVENT_MILESTONE_PAIR_PENDING), txn)
			if err == nil {
				err = db.Remove(db.AsKey(parentKey, db.KEY_EVENT_MILESTONE_PENDING), txn)
				_checkIncomingError(tx, err)
			}
		}

		snapTime := snapshot.GetSnapshotTimestamp(txn)
		futureTime := int(time.Now().Add(time.Duration(2) * time.Hour).Unix())
		maybeMilestonePair := isMaybeMilestone(tx) || isMaybeMilestonePair(tx)
		isOutsideOfTimeframe := !maybeMilestonePair && (tx.Timestamp > futureTime || snapTime >= tx.Timestamp)
		if isOutsideOfTimeframe && !db.Has(db.GetByteKey(tx.Bundle, db.KEY_PENDING_BUNDLE), txn) {
			// If the bundle is still not deleted, keep this TX. It might link to a pending TX...
			if db.CountByPrefix(db.GetByteKey(tx.Bundle, db.KEY_BUNDLE)) == 0 {
				//logs.Log.Debugf("Got TX timestamp outside of a valid time frame (%v-%v): %v",
				//	snapTime, futureTime, tx.Timestamp)
				removeTx()
				return nil
			}
		}

		if db.Has(db.AsKey(key, db.KEY_SNAPSHOTTED), txn) {
			_, err := requestIfMissing(tx.TrunkTransaction, incoming.IPAddressWithPort)
			_checkIncomingError(tx, err)
			_, err = requestIfMissing(tx.BranchTransaction, incoming.IPAddressWithPort)
			_checkIncomingError(tx, err)
			err = addPendingConfirmation(db.GetByteKey(tx.TrunkTransaction, db.KEY_EVENT_CONFIRMATION_PENDING), tx.Timestamp, txn)
			_checkIncomingError(tx, err)
			err = addPendingConfirmation(db.GetByteKey(tx.BranchTransaction, db.KEY_EVENT_CONFIRMATION_PENDING), tx.Timestamp, txn)
			_checkIncomingError(tx, err)
			/*logs.Log.Debugf("Got already snapshotted TX: %v, Value: %v",
			convert.BytesToTrytes(tx.Hash)[:81], tx.Value) */
			removeTx()
			return nil
		}

		if !db.Has(key, txn) {
			err := SaveTX(tx, incoming.Bytes, txn)
			_checkIncomingError(tx, err)
			if isMaybeMilestone(tx) {
				trunkBytesKey := db.GetByteKey(tx.TrunkTransaction, db.KEY_BYTES)
				err := db.PutBytes(db.AsKey(key, db.KEY_EVENT_MILESTONE_PENDING), trunkBytesKey, nil, txn)
				_checkIncomingError(tx, err)
				pendingMilestone = &PendingMilestone{key, trunkBytesKey}
			}
			_, err = requestIfMissing(tx.TrunkTransaction, incoming.IPAddressWithPort)
			_checkIncomingError(tx, err)
			_, err = requestIfMissing(tx.BranchTransaction, incoming.IPAddressWithPort)
			_checkIncomingError(tx, err)

			// EVENTS:

			pendingConfirmationKey := db.AsKey(key, db.KEY_PENDING_CONFIRMED)
			if db.Has(pendingConfirmationKey, txn) {
				err = db.Remove(pendingConfirmationKey, txn)
				_checkIncomingError(tx, err)
				err = addPendingConfirmation(db.AsKey(key, db.KEY_EVENT_CONFIRMATION_PENDING), tx.Timestamp, txn)
				_checkIncomingError(tx, err)
			}

			parentKey, err := db.GetBytes(db.AsKey(key, db.KEY_EVENT_MILESTONE_PAIR_PENDING), txn)
			if err == nil {
				pendingMilestone = &PendingMilestone{parentKey, db.AsKey(key, db.KEY_BYTES)}
			}

			// Re-broadcast new TX. Not always.
			// Here, it is actually possible to favor nearer neighbours!
			if utils.Random(0, 100) < P_BROADCAST {
				Broadcast(tx.Bytes, incoming.IPAddressWithPort)
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
		addPendingRequest(tx.Hash, 0, incoming.IPAddressWithPort, true)

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
