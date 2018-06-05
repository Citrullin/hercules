package api

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"convert"
	"db"
	"github.com/dgraph-io/badger"
	"time"
)

func getTrytes (request Request, c *gin.Context, t time.Time) {
	var trytes []string
	_ = db.DB.View(func(txn *badger.Txn) error {
		for _, hash := range request.Hashes {
			if !convert.IsTrytes(hash, 81) {
				ReplyError("Wrong hash trytes", c)
				return nil
			}
			b, err := db.GetBytes(db.GetByteKey(convert.TrytesToBytes(hash)[:49], db.KEY_BYTES), txn)
			if err == nil {
				trytes = append(trytes, convert.BytesToTrytes(b)[:2673])
			} else {
				trytes = append(trytes, "")
			}
		}
		return nil
	})
	c.JSON(http.StatusOK, gin.H{
		"trytes": trytes,
		"duration": getDuration(t),
	})
}