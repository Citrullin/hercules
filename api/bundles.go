package api

import (
	"net/http"
	"time"

	"../convert"
	"../db"
	"../logs"
	"../tangle"
	"../transaction"
	"github.com/dgraph-io/badger"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

func init() {
	addAPICall("storeTransactions", storeTransactions)
	addAPICall("broadcastTransactions", broadcastTransactions)
}

func storeTransactions(request Request, c *gin.Context, t time.Time) {
	storeAndBroadcastTransactions(request, c, false, t)
}

func broadcastTransactions(request Request, c *gin.Context, t time.Time) {
	storeAndBroadcastTransactions(request, c, true, t)
}

func storeAndBroadcastTransactions(request Request, c *gin.Context, broadcast bool, t time.Time) {
	var stored = 0
	var broadcasted = 0
	if request.Trytes == nil || len(request.Trytes) < 1 {
		ReplyError("No trytes provided", c)
		return
	}
	if !transaction.IsValidBundleTrytes(request.Trytes) {
		ReplyError("Invalid bundle", c)
		return
	}
	for _, trytes := range request.Trytes {
		err := db.DB.Update(func(txn *badger.Txn) (e error) {
			trits := convert.TrytesToTrits(trytes)
			bits := convert.TrytesToBytes(trytes)[:1604]
			tx := transaction.TritsToTX(&trits, bits)

			if tx.Value != 0 {
				balance, err := db.GetInt64(db.GetAddressKey(tx.Address, db.KEY_BALANCE), nil)
				if err != nil {
					addressTrytes := convert.BytesToTrytes(tx.Address)
					return errors.Errorf("Could not read address' balance. Address: %s Message: %v", addressTrytes, err)
				} else if balance <= 0 || balance < tx.Value {
					addressTrytes := convert.BytesToTrytes(tx.Address)

					// TODO: collect values from all TXs, map to addesses and check for the whole sum
					// This is as to prevent multiple partial transactions from the same address in the bundle
					return errors.Errorf("Insufficient balance. Sender address: %s Balance: %v Tx value: %v", addressTrytes, balance, tx.Value)
				}
			}

			if !db.Has(db.GetByteKey(tx.Hash, db.KEY_HASH), txn) {
				err := tangle.SaveTX(tx, &bits, txn)
				if err != nil {
					return err
				}
				stored++
			}
			if broadcast {
				tangle.Broadcast(tx.Bytes, "")
				broadcasted++
			}
			return nil
		})
		if err != nil {
			logs.Log.Warning("Error while saving transaction", err)
			ReplyError("Error encountered while saving a transaction", c)
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"stored":      stored,
		"broadcasted": broadcasted,
		"duration":    getDuration(t),
	})
}
