package tangle

import (
	"db"
	"errors"
	"server"
	"bytes"
	"github.com/dgraph-io/badger"
	"convert"
	"transaction"
	"crypt"
	"logs"
	"time"
)

func incomingRunner () {
	for raw := range srv.Incoming {
		data := raw.Msg[:1604]
		req := raw.Msg[1604:1650]
		msg := &Message{&data,&req, raw.Addr}

		incoming++

		db.Locker.Lock()
		db.Locker.Unlock()

		var pendingMilestone *PendingMilestone
		var pendingKey []byte
		var hash []byte
		var pendingTrytes = ""

		err := db.DB.Update(func(txn *badger.Txn) (e error) {
			defer func() {
				if err := recover(); err != nil {
					e = errors.New("Failed processing incoming TX!")
				}
			}()
			var fingerprint []byte
			var isTipRequest = bytes.Equal(*msg.Bytes, tipBytes)

			fingerprint = db.GetByteKey(*msg.Bytes, db.KEY_FINGERPRINT)
			if isTipRequest || !db.Has(fingerprint, txn) {
				if !isTipRequest {
					db.Put(fingerprint, true, &fingerprintTTL, txn)
				}
				incomingProcessed++
				trits := convert.BytesToTrits(*msg.Bytes)[:8019]
				tx := transaction.TritsToFastTX(&trits, *msg.Bytes)
				if !crypt.IsValidPoW(tx.Hash, MWM) {
					server.NeighborTrackingQueue <- &server.NeighborTrackingMessage{Addr: msg.Addr, Invalid: 1}
					return
				}

				var key = db.GetByteKey(tx.Hash, db.KEY_HASH)
				hash = tx.Hash

				pendingKey = db.AsKey(key, db.KEY_PENDING_HASH)
				pendingTrytes = convert.BytesToTrytes(tx.Hash)
				db.Remove(pendingKey, nil)
				PendingsLocker.Lock()
				_, ok := pendings[pendingTrytes]
				if ok {
					delete(pendings, pendingTrytes)
					//logs.Log.Warning("Time to reply", time.Now().Sub(time.Unix(t, 0)))
				}
				PendingsLocker.Unlock()

				// TODO: check if the TX is recent (younger than snapshot). Otherwise drop.

				if !isTipRequest && !db.Has(key, txn) {
					err := saveTX(tx, msg.Bytes, txn)
					_checkIncomingError(tx, err)
					if isMaybeMilestone(tx) {
						trunkHashKey := db.GetByteKey(tx.TrunkTransaction, db.KEY_BYTES)
						err := db.PutBytes(db.AsKey(key, db.KEY_EVENT_MILESTONE_PENDING), trunkHashKey,nil, txn)
						_checkIncomingError(tx, err)
						pendingMilestone = &PendingMilestone{key, trunkHashKey}
					}
					_, err = requestIfMissing(tx.TrunkTransaction, msg.Addr, txn)
					_checkIncomingError(tx, err)
					_, err = requestIfMissing(tx.BranchTransaction, msg.Addr, txn)
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
					// TODO: rework how responses are sent
					/*
					go func () {
						if utils.Random(0,100) < 10 {
							outgoingQueue <- getMessage(*msg.Bytes, nil, false, txn)
						}
					}()
					*/

					server.NeighborTrackingQueue <- &server.NeighborTrackingMessage{Addr: msg.Addr, New: 1}
					saved++
				} else if !isTipRequest {
					discarded++
				}

				// Add request

				tipRequest := isTipRequest || bytes.Equal(tx.Hash[:46], tipFastTX.Hash[:46]) || bytes.Equal(tx.Hash[:46], (*msg.Requested)[:46])
				req := make([]byte, 49)
				copy(req, *msg.Requested)
				queue, ok := requestReplyQueues[msg.Addr]
				if !ok {
					q := make(RequestQueue, maxQueueSize)
					queue = &q
					requestReplyQueues[msg.Addr] = queue
				}
				*queue <- &Request{req, tipRequest}
			}
			return nil
		})

		if err == nil {
			if pendingMilestone != nil {
				addPendingMilestoneToQueue(pendingMilestone)
			}
		} else {
			if pendingKey != nil {
				PendingsLocker.Lock()
				pendings[pendingTrytes] = time.Now().Unix()
				PendingsLocker.Unlock()
				db.Put(pendingKey, hash, nil, nil)
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