package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"../convert"
	"../db"
	"../db/ns"
)

func init() {
	addAPICall("getTrytes", getTrytes, mainAPICalls)
}

func getTrytes(request Request, c *gin.Context, t time.Time) {
	var trytes []interface{}
	db.Singleton.View(func(dbTx db.Transaction) error {
		for _, hash := range request.Hashes {
			if !convert.IsTrytes(hash, 81) {
				replyError("Wrong hash trytes", c)
				return nil
			}
			b, err := dbTx.GetBytes(ns.HashKey(convert.TrytesToBytes(hash)[:49], ns.NamespaceBytes))
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
