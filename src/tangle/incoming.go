package tangle

import (
	"db"
	"utils"
	"server"
	"bytes"
	"github.com/dgraph-io/badger"
	"convert"
	"transaction"
	"crypt"
)

func incomingRunner () {
	for raw := range srv.Incoming {
		data := raw.Msg[:1604]
		req := raw.Msg[1604:1650]
		msg := &Message{&data,&req, raw.Addr}

		incoming++

		db.Locker.Lock()
		db.Locker.Unlock()

		_ = db.DB.Update(func(txn *badger.Txn) error {
			var fingerprint []byte
			var has = bytes.Equal(*msg.Bytes, tipBytes)

			if !has {
				fingerprint = db.GetByteKey(*msg.Bytes, db.KEY_FINGERPRINT)
				if !db.Has(fingerprint, txn) {
					db.Put(fingerprint, true, &fingerprintTTL, txn)
					incomingQueue <- msg
					incomingProcessed++
				}
			}
			return nil
		})
	}
}

func listenToIncoming () {
	for msg := range incomingQueue {
		trits := convert.BytesToTrits(*msg.Bytes)[:8019]
		tx := transaction.TritsToFastTX(&trits, *msg.Bytes)
		if !crypt.IsValidPoW(tx.Hash, MWM) {
			server.NeighborTrackingQueue <- &server.NeighborTrackingMessage{Addr: msg.Addr, Invalid: 1}
			continue
		}

		db.Locker.Lock()
		db.Locker.Unlock()

		_ = db.DB.Update(func(txn *badger.Txn) error {
			db.Remove(db.GetByteKey(tx.Hash, db.KEY_PENDING), txn)
			db.Remove(db.GetByteKey(tx.Hash, db.KEY_PENDING_HASH), txn)

			// TODO: check if the TX is recent (younger than snapshot). Otherwise drop.

			if !db.Has(db.GetByteKey(tx.Hash, db.KEY_HASH), txn) {
				saveTX(tx, msg.Bytes, txn)
				if isMaybeMilestone(tx) {
					db.PutBytes(db.GetByteKey(tx.Hash, db.KEY_EVENT_MILESTONE_PENDING),
						db.GetByteKey(tx.TrunkTransaction, db.KEY_BYTES),nil, txn)
					//log.Println("ADDED KEY_EVENT_MILESTONE_PENDING", db.GetByteKey(tx.TrunkTransaction, db.KEY_BYTES))
				}
				requestIfMissing(tx.TrunkTransaction, msg.Addr, txn)
				requestIfMissing(tx.BranchTransaction, msg.Addr, txn)

				// EVENTS:

				pendingConfirmationKey := db.GetByteKey(tx.Hash, db.KEY_PENDING_CONFIRMED)
				if db.Has(pendingConfirmationKey, txn) {
					db.Remove(pendingConfirmationKey, txn)
					db.Put(db.GetByteKey(tx.Hash, db.KEY_EVENT_CONFIRMATION_PENDING), "", nil, txn)
				}

				// Re-broadcast new TX. Not always.
				go func () {
					if utils.Random(0,100) < 10 {
						outgoingQueue <- getMessage(*msg.Bytes, nil, false, txn)
					}
				}()
				server.NeighborTrackingQueue <- &server.NeighborTrackingMessage{Addr: msg.Addr, New: 1}
				saved++
			} else {
				discarded++
			}

			// Add request

			if len(requestReplyQueue) < 1000 {
				tipRequest := bytes.Equal(tx.Hash[:46], tipFastTX.Hash[:46]) || bytes.Equal(tx.Hash[:46], (*msg.Requested)[:46])
				req := make([]byte, 49)
				copy(req, *msg.Requested)
				requestReplyQueue <- &Request{req, msg.Addr, tipRequest}
			}

			return nil

		})
	}
}
