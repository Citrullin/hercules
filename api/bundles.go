package api

import (
	"net/http"
	"time"
	"github.com/gin-gonic/gin"
	"github.com/dgraph-io/badger"
	"gitlab.com/semkodev/hercules.go/convert"
	"gitlab.com/semkodev/hercules.go/db"
	"gitlab.com/semkodev/hercules.go/tangle"
	"gitlab.com/semkodev/hercules.go/transaction"
	"gitlab.com/semkodev/hercules.go/logs"
)

func storeTransactions (request Request, c *gin.Context, broadcast bool, t time.Time) {
	var stored = 0
	var broadcasted = 0
	if request.Trytes == nil || len(request.Trytes) < 1 {
		ReplyError("No trytes provided", c)
		return
	}
	if !tangle.IsValidBundle(request.Trytes) {
		ReplyError("Invalid bundle", c)
		return
	}
	for _, trytes := range request.Trytes {
		err := db.DB.Update(func(txn *badger.Txn) (e error) {
			trits := convert.TrytesToTrits(trytes)
			bits := convert.TrytesToBytes(trytes)[:1604]
			tx := transaction.TritsToTX(&trits, bits)
			if !db.Has(db.GetByteKey(tx.Hash, db.KEY_HASH), txn) {
				err := tangle.SaveTX(tx, &bits, txn)
				if err != nil {
					return err
				}
				if broadcast {
					tangle.Broadcast(tx.Hash)
					broadcasted++
				}
				stored++
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
		"stored": stored,
		"broadcasted": broadcasted,
		"duration": getDuration(t),
	})
}
