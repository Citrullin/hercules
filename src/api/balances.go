package api

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"convert"
	"db"
	"tangle"
)

func getBalances (request Request, c *gin.Context) {
	if request.Addresses != nil {
		var balances []int64
		for _, address := range request.Addresses {
			// TODO: check trytes?
			addressBytes := convert.TrytesToBytes(address)
			if addressBytes == nil {
				balances = append(balances, 0)
				continue
			}
			balance, err := db.GetInt64(db.GetByteKey(addressBytes, db.KEY_BALANCE), nil)
			if err != nil {
				balances = append(balances, 0)
				continue
			}
			balances = append(balances, balance)
		}
		c.JSON(http.StatusOK, gin.H{
			"balances": balances,
			"duration": 0,
			"milestone": convert.BytesToTrytes(tangle.LatestMilestone.TX.Hash)[:81],
			"milestoneIndex": tangle.LatestMilestone.Index,
		})
	}
}