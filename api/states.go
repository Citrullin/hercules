package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"gitlab.com/semkodev/hercules/convert"
	"gitlab.com/semkodev/hercules/db"
	"gitlab.com/semkodev/hercules/db/ns"
)

func init() {
	addAPICall("getInclusionStates", getInclusionStates, mainAPICalls)
	addAPICall("wereAddressesSpentFrom", wereAddressesSpentFrom, mainAPICalls)
}

func getInclusionStates(request Request, c *gin.Context, ts time.Time) {
	var states = []bool{}
	db.Singleton.View(func(dbTx db.Transaction) error {
		for _, hash := range request.Transactions {
			if !convert.IsTrytes(hash, 81) {
				replyError("Wrong hash trytes", c)
				return nil
			}
			states = append(states, dbTx.HasKey(ns.HashKey(convert.TrytesToBytes(hash)[:49], ns.NamespaceConfirmed)))
		}
		return nil
	})
	c.JSON(http.StatusOK, gin.H{
		"states":   states,
		"duration": getDuration(ts),
	})
}

func wereAddressesSpentFrom(request Request, c *gin.Context, ts time.Time) {
	var states = []bool{}
	db.Singleton.View(func(dbTx db.Transaction) error {
		for _, hash := range request.Addresses {
			if !convert.IsTrytes(hash, 81) {
				replyError("Wrong hash trytes", c)
				return nil
			}
			states = append(states, dbTx.HasKey(ns.AddressKey(convert.TrytesToBytes(hash)[:49], ns.NamespaceSpent)))
		}
		return nil
	})
	c.JSON(http.StatusOK, gin.H{
		"states":   states,
		"duration": getDuration(ts),
	})
}
