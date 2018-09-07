package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"../convert"
	"../db"
	"../db/coding"
	"../db/ns"
)

func init() {
	addAPICall("getTrytes", getTrytes)
}

func getTrytes(request Request, c *gin.Context, t time.Time) {
	var trytes []interface{}
	db.Singleton.View(func(tx db.Transaction) error {
		for _, hash := range request.Hashes {
			if !convert.IsTrytes(hash, 81) {
				ReplyError("Wrong hash trytes", c)
				return nil
			}
			b, err := coding.GetBytes(tx, ns.HashKey(convert.TrytesToBytes(hash)[:49], ns.NamespaceBytes))
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
			"trytes":   make([]string, 0),
			"duration": getDuration(t),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"trytes":   trytes,
		"duration": getDuration(t),
	})
}
