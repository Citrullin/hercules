package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"

	"../convert"
	"../db"
	"../db/coding"
	"../db/ns"
	"../logs"
	"../tangle"
	"../transaction"
)

func init() {
	addAPICall("storeTransactions", storeTransactions, mainAPICalls)
	addAPICall("broadcastTransactions", broadcastTransactions, mainAPICalls)
}

func storeTransactions(request Request, c *gin.Context, ts time.Time) {
	storeAndBroadcastTransactions(request, c, false, ts)
}

func broadcastTransactions(request Request, c *gin.Context, ts time.Time) {
	storeAndBroadcastTransactions(request, c, true, ts)
}

func storeAndBroadcastTransactions(request Request, c *gin.Context, broadcast bool, ts time.Time) {
	var stored = 0
	var broadcasted = 0
	if request.Trytes == nil || len(request.Trytes) < 1 {
		replyError("No trytes provided", c)
		return
	}
	if !transaction.IsValidBundleTrytes(request.Trytes) {
		replyError("Invalid bundle", c)
		return
	}
	for _, trytes := range request.Trytes {
		err := db.Singleton.Update(func(dbTx db.Transaction) error {
			trits := convert.TrytesToTrits(trytes)
			bits := convert.TrytesToBytes(trytes)[:1604]
			tx := transaction.TritsToTX(&trits, bits)

			// tx.Address is the receiving address
			// only when the transaction value is negative we should check for balance in the receiving address
			if tx.Value < 0 {
				balance, err := coding.GetInt64(dbTx, ns.AddressKey(tx.Address, ns.NamespaceBalance))
				if err != nil {
					addressTrytes := convert.BytesToTrytes(tx.Address)
					return errors.Errorf("Could not read address' balance. Address: %s Message: %s", addressTrytes, err)
				} else if balance <= 0 || balance < tx.Value {
					addressTrytes := convert.BytesToTrytes(tx.Address)

					// TODO: collect values from all TXs, map to addesses and check for the whole sum
					// This is as to prevent multiple partial transactions from the same address in the bundle
					return errors.Errorf("Insufficient balance. Address: %s Balance: %v Tx value: %v", addressTrytes, balance, tx.Value)
				}
			}

			if !dbTx.HasKey(ns.HashKey(tx.Hash, ns.NamespaceHash)) {
				err := tangle.SaveTX(tx, &bits, dbTx)
				if err != nil {
					return err
				}
				stored++
			}
			if broadcast {
				tangle.Broadcast(tx.Bytes, nil)
				broadcasted++
			}
			return nil
		})
		if err != nil {
			logs.Log.Warning("Error while saving transaction. ", err)
			replyError("Error encountered while saving a transaction", c)
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"stored":      stored,
		"broadcasted": broadcasted,
		"duration":    getDuration(ts),
	})
}
