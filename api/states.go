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
			states = append(states, tx.HasKey(ns.HashKey(convert.TrytesToBytes(hash)[:49], ns.NamespaceConfirmed)))
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
			states = append(states, tx.HasKey(ns.AddressKey(convert.TrytesToBytes(hash)[:49], ns.NamespaceSpent)))
		}
		return nil
	})
	c.JSON(http.StatusOK, gin.H{
		"states":   states,
		"duration": getDuration(t),
	})
}
