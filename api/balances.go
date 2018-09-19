package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"gitlab.com/semkodev/hercules/convert"
	"gitlab.com/semkodev/hercules/db"
	"gitlab.com/semkodev/hercules/db/coding"
	"gitlab.com/semkodev/hercules/db/ns"
	"gitlab.com/semkodev/hercules/tangle"
)

func init() {
	addAPICall("getBalances", getBalances, mainAPICalls)
	addAPICall("listAllAccounts", listAllAccounts, mainAPICalls)
}

func getBalances(request Request, c *gin.Context, ts time.Time) {
	if request.Addresses == nil {
		return
	}

	var balances = []int64{}
	for _, address := range request.Addresses {
		if !convert.IsTrytes(address, 81) {
			replyError("Wrong trytes", c)
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
		"duration":       getDuration(ts),
		"milestone":      convert.BytesToTrytes(tangle.LatestMilestone.TX.Hash)[:81],
		"milestoneIndex": tangle.LatestMilestone.Index,
	})
}

func listAllAccounts(request Request, c *gin.Context, ts time.Time) {
	var accounts = make(map[string]interface{})
	db.Singleton.View(func(dbTx db.Transaction) error {
		return coding.ForPrefixInt64(dbTx, ns.Prefix(ns.NamespaceBalance), false, func(key []byte, value int64) (bool, error) {
			if value == 0 {
				return true, nil
			}

			accounts[convert.BytesToTrytes(key[1:])[:81]] = value

			return true, nil
		})
	})
	c.JSON(http.StatusOK, gin.H{
		"accounts":       accounts,
		"duration":       getDuration(ts),
		"milestone":      convert.BytesToTrytes(tangle.LatestMilestone.TX.Hash)[:81],
		"milestoneIndex": tangle.LatestMilestone.Index,
	})
}
