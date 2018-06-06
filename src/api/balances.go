package api

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"convert"
	"db"
	"tangle"
	"time"
)

func getBalances (request Request, c *gin.Context, t time.Time) {
	if request.Addresses != nil {
		var balances []int64
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
			balance, err := db.GetInt64(db.GetByteKey(addressBytes, db.KEY_BALANCE), nil)
			if err != nil {
				balances = append(balances, 0)
				continue
			}
			balances = append(balances, balance)
		}
		c.JSON(http.StatusOK, gin.H{
			"balances": balances,
			"duration": getDuration(t),
			"milestone": convert.BytesToTrytes(tangle.LatestMilestone.TX.Hash)[:81],
			"milestoneIndex": tangle.LatestMilestone.Index,
		})
	}
}