package api

import (
	"net/http"
	"time"

	"../convert"
	"../db"
	"github.com/gin-gonic/gin"
)

func init() {
	addAPICall("getInclusionStates", getInclusionStates)
	addAPICall("wereAddressesSpentFrom", wereAddressesSpentFrom)
}

func getInclusionStates(request Request, c *gin.Context, t time.Time) {
	var states = []bool{}
	db.Singleton.View(func(tx db.Transaction) error {
		for _, hash := range request.Transactions {
			if !convert.IsTrytes(hash, 81) {
				ReplyError("Wrong hash trytes", c)
				return nil
			}
			states = append(states, tx.HasKey(db.GetByteKey(convert.TrytesToBytes(hash)[:49], db.KEY_CONFIRMED)))
		}
		return nil
	})
	c.JSON(http.StatusOK, gin.H{
		"states":   states,
		"duration": getDuration(t),
	})
}

func wereAddressesSpentFrom(request Request, c *gin.Context, t time.Time) {
	var states = []bool{}
	db.Singleton.View(func(tx db.Transaction) error {
		for _, hash := range request.Addresses {
			if !convert.IsTrytes(hash, 81) {
				ReplyError("Wrong hash trytes", c)
				return nil
			}
			states = append(states, tx.HasKey(db.GetAddressKey(convert.TrytesToBytes(hash)[:49], db.KEY_SPENT)))
		}
		return nil
	})
	c.JSON(http.StatusOK, gin.H{
		"states":   states,
		"duration": getDuration(t),
	})
}
