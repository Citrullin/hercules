package tangle

import (
	"db"
	"server"
	"bytes"
	"github.com/dgraph-io/badger"
	"convert"
	"transaction"
	"crypt"
	"logs"
	"utils"
	"time"
	"snapshot"
)

const P_TIP_REPLY = 20

func incomingRunner () {
	for raw := range srv.Incoming {
		data := raw.Msg[:1604]
		req := raw.Msg[1604:1650]
		msg := &Message{&data,&req, raw.Addr}
		var tx *transaction.FastTX
		var isTipRequest = false

		incoming++

		db.Locker.Lock()
		db.Locker.Unlock()

		err := db.DB.Update(func(txn *badger.Txn) (e error) {
			isTipRequest = bytes.Equal(*msg.Bytes, tipBytes)

			trits := convert.BytesToTrits(*msg.Bytes)[:8019]
			tx = transaction.TritsToTX(&trits, *msg.Bytes)
			if !crypt.IsValidPoW(tx.Hash, MWM) {
				server.NeighborTrackingQueue <- &server.NeighborTrackingMessage{Addr: msg.Addr, Invalid: 1}
				return nil
			}
			tipRequest := isTipRequest || bytes.Equal(tx.Hash[:46], tipFastTX.Hash[:46]) || bytes.Equal(tx.Hash[:46], (*msg.Requested)[:46])

			if !tipRequest || utils.Random(0, 100) < P_TIP_REPLY {
				req := make([]byte, 49)
				copy(req, *msg.Requested)
				replyLocker.Lock()
				identifier, _ := server.GetNeighborByAddress(msg.Addr)
				queue, ok := replyQueues[identifier]
				if !ok {
					q := make(RequestQueue, maxQueueSize)
					queue = &q
					replyQueues[identifier] = queue
				}
				replyLocker.Unlock()
				*queue <- &Request{req, tipRequest}
			}
			return nil
		})
		if err == nil && !isTipRequest && tx != nil {
			incomingProcessed++
			txQueue <- &IncomingTX{tx, raw.Addr, msg.Bytes}
		}
	}
}

func processIncomingTX (incoming *IncomingTX) {
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

		snapTime := snapshot.GetSnapshotTimestamp(txn)
		if tx.Timestamp != 0 && snapTime >= tx.Timestamp && !db.Has(db.GetByteKey(tx.Bundle, db.KEY_PENDING_BUNDLE), txn)  {
			// If the bundle is still not deleted, keep this TX. It might link to a pending TX...
			if db.CountByPrefix(db.GetByteKey(tx.Bundle, db.KEY_BUNDLE)) == 0 {
				logs.Log.Warningf("Got old TX older than snapshot (skipping): %v vs %v, Value: %v",
					tx.Timestamp, snapTime, tx.Value)
				logs.Log.Warning("Skipping this TX:", convert.BytesToTrytes(tx.Hash))
				db.Remove(db.AsKey(key, db.KEY_PENDING_CONFIRMED), txn)
				db.Remove(db.AsKey(key, db.KEY_EVENT_CONFIRMATION_PENDING), txn)
				db.Remove(db.AsKey(key, db.KEY_EVENT_MILESTONE_PAIR_PENDING), txn)
				parentKey, err := db.GetBytes(db.AsKey(key, db.KEY_EVENT_MILESTONE_PAIR_PENDING), txn)
				if err == nil {
					db.Remove(db.AsKey(parentKey, db.KEY_EVENT_MILESTONE_PENDING), txn)
				}
				return nil
			}
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
			_, err = requestIfMissing(tx.TrunkTransaction, incoming.Addr, txn)
			_checkIncomingError(tx, err)
			_, err = requestIfMissing(tx.BranchTransaction, incoming.Addr, txn)
			_checkIncomingError(tx, err)

			// EVENTS:

			pendingConfirmationKey := db.AsKey(key, db.KEY_PENDING_CONFIRMED)
			if db.Has(pendingConfirmationKey, txn) {
				err = db.Remove(pendingConfirmationKey, txn)
				_checkIncomingError(tx, err)
				err = db.Put(db.AsKey(key, db.KEY_EVENT_CONFIRMATION_PENDING), "", nil, txn)
				_checkIncomingError(tx, err)
			}

			parentKey, err := db.GetBytes(db.AsKey(key, db.KEY_EVENT_MILESTONE_PAIR_PENDING), txn)
			if err == nil {
				pendingMilestone = &PendingMilestone{parentKey, db.AsKey(key, db.KEY_BYTES)}
			}

			// Re-broadcast new TX. Not always.
			// Here, it is actually possible to favor nearer neighbours!
			if utils.Random(0, 100) < 5 {
				Broadcast(tx.Hash)
			}

			server.NeighborTrackingQueue <- &server.NeighborTrackingMessage{Addr: incoming.Addr, New: 1}
			saved++
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
		if pendingKey != nil {
			nowUnix := time.Now().Unix()
			db.Put(pendingKey, hash, nil, nil)
			db.Put(db.AsKey(pendingKey, db.KEY_PENDING_TIMESTAMP), nowUnix, nil, nil)
			addPendingRequest(hash, int(nowUnix), incoming.Addr)
		}
		if err == badger.ErrConflict {
			processIncomingTX(incoming)
		}
	}
}

func _checkIncomingError(tx *transaction.FastTX, err error) {
	if err != nil {
		logs.Log.Errorf("Failed processing TX %v", convert.BytesToTrytes(tx.Hash)[:81], err)
		panic(err)
	}
}