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
)

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
			var fingerprint []byte
			isTipRequest = bytes.Equal(*msg.Bytes, tipBytes)

			fingerprint = db.GetByteKey(*msg.Bytes, db.KEY_FINGERPRINT)
			if isTipRequest || !db.Has(fingerprint, txn) {
				if !isTipRequest {
					db.Put(fingerprint, true, &fingerprintTTL, txn)
				}
				trits := convert.BytesToTrits(*msg.Bytes)[:8019]
				tx = transaction.TritsToFastTX(&trits, *msg.Bytes)
				if !crypt.IsValidPoW(tx.Hash, MWM) {
					server.NeighborTrackingQueue <- &server.NeighborTrackingMessage{Addr: msg.Addr, Invalid: 1}
					return nil
				}
				tipRequest := isTipRequest || bytes.Equal(tx.Hash[:46], tipFastTX.Hash[:46]) || bytes.Equal(tx.Hash[:46], (*msg.Requested)[:46])

				req := make([]byte, 49)
				copy(req, *msg.Requested)
				replyLocker.Lock()
				queue, ok := replyQueues[msg.Addr]
				if !ok {
					q := make(RequestQueue, maxQueueSize)
					queue = &q
					replyQueues[msg.Addr] = queue
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
		if len(txQueue) > 10 {
			time.Sleep(time.Duration(len(txQueue)) * time.Millisecond)
		}
	}
}

func processIncomingTX (incoming *IncomingTX) {
	tx := incoming.TX
	var pendingMilestone *PendingMilestone
	var hash []byte
	var pendingKey []byte
	err := db.DB.Update(func(txn *badger.Txn) (e error) {
		var key = db.GetByteKey(tx.Hash, db.KEY_HASH)
		hash = tx.Hash

		pendingKey = db.AsKey(key, db.KEY_PENDING_HASH)
		db.Remove(pendingKey, txn)
		db.Remove(db.AsKey(key, db.KEY_PENDING_TIMESTAMP), txn)

		// TODO: check if the TX is recent (younger than snapshot). Otherwise drop.

		if !db.Has(key, txn) {
			err := saveTX(tx, incoming.Bytes, txn)
			_checkIncomingError(tx, err)
			if isMaybeMilestone(tx) {
				trunkHashKey := db.GetByteKey(tx.TrunkTransaction, db.KEY_BYTES)
				err := db.PutBytes(db.AsKey(key, db.KEY_EVENT_MILESTONE_PENDING), trunkHashKey, nil, txn)
				_checkIncomingError(tx, err)
				pendingMilestone = &PendingMilestone{key, trunkHashKey}
			}
			// TODO: discard pending, whose parent has been snapshotted
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
				pendingMilestone = &PendingMilestone{parentKey, key}
			}

			// Re-broadcast new TX. Not always.
			// Here, it is actually possible to favor nearer neighbours!
			if utils.Random(0, 100) < 5 {
				replyLocker.RLock()
				for addr, queue := range replyQueues {
					if addr != incoming.Addr && len(*queue) < 1000 {
						*queue <- &Request{tx.Hash, false}
					}
				}
				replyLocker.RUnlock()
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
			db.Put(pendingKey, hash, nil, nil)
			db.Put(db.AsKey(pendingKey, db.KEY_PENDING_TIMESTAMP), time.Now().Unix(), nil, nil)
		}
	}
}

func _checkIncomingError(tx *transaction.FastTX, err error) {
	if err != nil {
		logs.Log.Errorf("Failed processing TX %v", convert.BytesToTrytes(tx.Hash)[:81], err)
		panic(err)
	}
}