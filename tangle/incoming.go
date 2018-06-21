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

		var isJustTipRequest = bytes.Equal(data, tipBytes)

		if isJustTipRequest && utils.Random(0, 100) < P_TIP_REPLY {
			continue
		}

		trits := convert.BytesToTrits(data)[:8019]
		var tx = transaction.TritsToTX(&trits, data)

		if !isJustTipRequest {
			if !crypt.IsValidPoW(tx.Hash, MWM) {
				server.NeighborTrackingQueue <- &server.NeighborTrackingMessage{IPAddressWithPort: raw.IPAddressWithPort, Invalid: 1}
			} else {
				processIncomingTX(IncomingTX{TX: tx, IPAddressWithPort: raw.IPAddressWithPort, Bytes: &data})
				incomingProcessed++
			}
		}

		// Pause for a while without responding to prevent flooding
		if len(srv.Incoming) > maxIncoming {
			continue
		}

		var reply []byte = nil
		var isTipRequest = isJustTipRequest || bytes.Equal(tx.Hash, req)

		if !isTipRequest {
			reply, _ = db.GetBytes(db.GetByteKey(req, db.KEY_BYTES), nil)
		}

		request := getSomeRequestByIPAddressWithPort(raw.IPAddressWithPort)
		// It's a specific (not tip) request that we do not have.
		// Avoid creating a tip request on our own
		if !isTipRequest && request == nil && reply == nil {
			continue
		}

		sendReply(getMessage(reply, request, request == nil, raw.IPAddressWithPort, nil))
	}
}

func processIncomingTX(incoming IncomingTX) {
	tx := incoming.TX
	var pendingMilestone *PendingMilestone
	var hash []byte
	var pendingKey []byte
	err := db.DB.Update(func(txn *badger.Txn) (e error) {
		// TODO: catch error defer here
		var key = db.GetByteKey(tx.Hash, db.KEY_HASH)
		hash = tx.Hash

		pendingKey = db.AsKey(key, db.KEY_PENDING_HASH)
		db.Remove(pendingKey, txn)
		db.Remove(db.AsKey(key, db.KEY_PENDING_TIMESTAMP), txn)
		removePendingRequest(tx.Hash)

		removeTx := func() {
			logs.Log.Debugf("Skipping this TX: %v", convert.BytesToTrytes(tx.Hash)[:81])
			db.Remove(db.AsKey(key, db.KEY_PENDING_CONFIRMED), txn)
			db.Remove(db.AsKey(key, db.KEY_EVENT_CONFIRMATION_PENDING), txn)
			db.Remove(db.AsKey(key, db.KEY_EVENT_MILESTONE_PAIR_PENDING), txn)
			err := db.Put(db.AsKey(key, db.KEY_EDGE), true, nil, txn)
			_checkIncomingError(tx, err)
			parentKey, err := db.GetBytes(db.AsKey(key, db.KEY_EVENT_MILESTONE_PAIR_PENDING), txn)
			if err == nil {
				db.Remove(db.AsKey(parentKey, db.KEY_EVENT_MILESTONE_PENDING), txn)
			}
		}

		snapTime := snapshot.GetSnapshotTimestamp(txn)
		if tx.Timestamp != 0 && snapTime >= tx.Timestamp && !db.Has(db.GetByteKey(tx.Bundle, db.KEY_PENDING_BUNDLE), txn) {
			// If the bundle is still not deleted, keep this TX. It might link to a pending TX...
			if db.CountByPrefix(db.GetByteKey(tx.Bundle, db.KEY_BUNDLE)) == 0 {
				logs.Log.Debugf("Got old TX older than snapshot (skipping) - Tx timestamp: '%v' - Snapshot timestamp: '%v' - Tx value: %v",
					tx.Timestamp, snapTime, tx.Value)
				removeTx()
				return nil
			}
		}

		if db.Has(db.AsKey(key, db.KEY_SNAPSHOTTED), txn) {
			_, err := requestIfMissing(tx.TrunkTransaction, incoming.IPAddressWithPort, txn)
			_checkIncomingError(tx, err)
			_, err = requestIfMissing(tx.BranchTransaction, incoming.IPAddressWithPort, txn)
			_checkIncomingError(tx, err)
			err = db.Put(db.GetByteKey(tx.TrunkTransaction, db.KEY_EVENT_CONFIRMATION_PENDING), tx.Timestamp, nil, txn)
			_checkIncomingError(tx, err)
			err = db.Put(db.GetByteKey(tx.BranchTransaction, db.KEY_EVENT_CONFIRMATION_PENDING), tx.Timestamp, nil, txn)
			_checkIncomingError(tx, err)
			logs.Log.Debugf("Got already snapshotted TX: %v, Value: %v",
				convert.BytesToTrytes(tx.Hash)[:81], tx.Value)
			removeTx()
			return nil
		}

		if !db.Has(key, txn) {
			err := SaveTX(tx, incoming.Bytes, txn)
			_checkIncomingError(tx, err)
			db.LatestTransactionTimestamp = tx.Timestamp
			if isMaybeMilestone(tx) {
				trunkBytesKey := db.GetByteKey(tx.TrunkTransaction, db.KEY_BYTES)
				err := db.PutBytes(db.AsKey(key, db.KEY_EVENT_MILESTONE_PENDING), trunkBytesKey, nil, txn)
				_checkIncomingError(tx, err)
				pendingMilestone = &PendingMilestone{key, trunkBytesKey}
			}
			_, err = requestIfMissing(tx.TrunkTransaction, incoming.IPAddressWithPort, txn)
			_checkIncomingError(tx, err)
			_, err = requestIfMissing(tx.BranchTransaction, incoming.IPAddressWithPort, txn)
			_checkIncomingError(tx, err)

			// EVENTS:

			pendingConfirmationKey := db.AsKey(key, db.KEY_PENDING_CONFIRMED)
			if db.Has(pendingConfirmationKey, txn) {
				err = db.Remove(pendingConfirmationKey, txn)
				_checkIncomingError(tx, err)
				err = db.Put(db.AsKey(key, db.KEY_EVENT_CONFIRMATION_PENDING), tx.Timestamp, nil, txn)
				_checkIncomingError(tx, err)
			}

			parentKey, err := db.GetBytes(db.AsKey(key, db.KEY_EVENT_MILESTONE_PAIR_PENDING), txn)
			if err == nil {
				pendingMilestone = &PendingMilestone{parentKey, db.AsKey(key, db.KEY_BYTES)}
			}

			// Re-broadcast new TX. Not always.
			// Here, it is actually possible to favor nearer neighbours!
			if utils.Random(0, 100) < P_BROADCAST {
				Broadcast(tx.Bytes)
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
		if !lowEndDevice && err == badger.ErrConflict {
			atomic.AddInt64(&totalTransactions, -1)
			server.NeighborTrackingQueue <- &server.NeighborTrackingMessage{IPAddressWithPort: incoming.IPAddressWithPort, New: -1}
			processIncomingTX(incoming)
		} else {
			if pendingKey != nil {
				nowUnix := time.Now().Unix()
				db.Put(pendingKey, hash, nil, nil)
				db.Put(db.AsKey(pendingKey, db.KEY_PENDING_TIMESTAMP), nowUnix, nil, nil)
				addPendingRequest(hash, int(nowUnix), incoming.IPAddressWithPort)
			}
		}
	}
}

func _checkIncomingError(tx *transaction.FastTX, err error) {
	if err != nil {
		logs.Log.Errorf("Failed processing TX %v", convert.BytesToTrytes(tx.Hash)[:81], err)
		panic(err)
	}
}
