package api

import (
	"bytes"
	"encoding/gob"
	"net/http"
	"time"

	"../convert"
	"../db"
	"../logs"
	"../tangle"
	"github.com/gin-gonic/gin"
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
		balance, err := db.Singleton.GetInt64(db.GetAddressKey(addressBytes, db.KEY_BALANCE))
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
		return tx.ForPrefix([]byte{db.KEY_BALANCE}, true, func(key, value []byte) (bool, error) {
			var v int64 = 0
			if err := gob.NewDecoder(bytes.NewBuffer(value)).Decode(&v); err != nil {
				logs.Log.Error("Could not parse a snapshot value from database!", err)
				return false, err
			}

			if v == 0 {
				return true, nil
			}

			accounts[convert.BytesToTrytes(key[1:])[:81]] = v

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
