package api

import (
	"net/http"
	"time"
	"github.com/gin-gonic/gin"
	"github.com/dgraph-io/badger"
	"gitlab.com/semkodev/hercules.go/convert"
	"gitlab.com/semkodev/hercules.go/db"
)

func getTrytes (request Request, c *gin.Context, t time.Time) {
	var trytes []interface{}
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
				trytes = append(trytes, false)
			}
		}
		return nil
	})

	if trytes == nil {
		c.JSON(http.StatusOK, gin.H{
			"trytes": make([]string, 0),
			"duration": getDuration(t),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"trytes": trytes,
		"duration": getDuration(t),
	})
}