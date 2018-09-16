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
			t := transaction.TritsToTX(&trits, bits)

			// t.Address is the receiving address
			// only when the transaction value is negative we should check for balance in the receiving address
			if t.Value < 0 {
				balance, err := coding.GetInt64(dbTx, ns.AddressKey(t.Address, ns.NamespaceBalance))
				if err != nil {
					addressTrytes := convert.BytesToTrytes(t.Address)
					return errors.Errorf("Could not read address' balance. Address: %s Message: %s", addressTrytes, err)
				} else if balance <= 0 || balance < t.Value {
					addressTrytes := convert.BytesToTrytes(t.Address)

					// TODO: collect values from all TXs, map to addesses and check for the whole sum
					// This is as to prevent multiple partial transactions from the same address in the bundle
					return errors.Errorf("Insufficient balance. Address: %s Balance: %v Tx value: %v", addressTrytes, balance, t.Value)
				}
			}

			if !dbTx.HasKey(ns.HashKey(t.Hash, ns.NamespaceHash)) {
				err := tangle.SaveTX(t, &bits, dbTx)
				if err != nil {
					return err
				}
				stored++
			}
			if broadcast {
				tangle.Broadcast(t.Bytes, nil)
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
		"duration":    getDuration(t),
	})
}
