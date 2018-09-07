package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"../convert"
	"../db"
	"../db/coding"
	"../db/ns"
	"../tangle"
)

func init() {
	addAPICall("getBalances", getBalances)
	addAPICall("listAllAccounts", listAllAccounts)
}

func getBalances(request Request, c *gin.Context, t time.Time) {
	if request.Addresses == nil {
		return
	}

	var balances = []int64{}
	for _, address := range request.Addresses {
		if !convert.IsTrytes(address, 81) {
			ReplyError("Wrong trytes", c)
			return
		}
		addressBytes := convert.TrytesToBytes(address)[:49]
		if addressBytes == nil {
			balances = append(balances, 0)
			continue
		}
		balance, err := coding.GetInt64(db.Singleton, ns.AddressKey(addressBytes, ns.NamespaceBalance))
		if err != nil {
			balances = append(balances, 0)
			continue
		}
		balances = append(balances, balance)
	}
	c.JSON(http.StatusOK, gin.H{
		"balances":       balances,
		"duration":       getDuration(t),
		"milestone":      convert.BytesToTrytes(tangle.LatestMilestone.TX.Hash)[:81],
		"milestoneIndex": tangle.LatestMilestone.Index,
	})
}

func listAllAccounts(request Request, c *gin.Context, t time.Time) {
	var accounts = make(map[string]interface{})
	db.Singleton.View(func(tx db.Transaction) error {
		return coding.ForPrefixInt64(tx, []byte{ns.NamespaceBalance}, false, func(key []byte, value int64) (bool, error) {
			if value == 0 {
				return true, nil
			}

			accounts[convert.BytesToTrytes(key[1:])[:81]] = value

			return true, nil
		})
	})
	c.JSON(http.StatusOK, gin.H{
		"accounts":       accounts,
		"duration":       getDuration(t),
		"milestone":      convert.BytesToTrytes(tangle.LatestMilestone.TX.Hash)[:81],
		"milestoneIndex": tangle.LatestMilestone.Index,
	})
}
