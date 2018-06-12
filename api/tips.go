package api

import (
	"net/http"
	"time"
	"github.com/gin-gonic/gin"
	"gitlab.com/semkodev/hercules/tangle"
	"gitlab.com/semkodev/hercules/convert"
)

func getTips (request Request, c *gin.Context, t time.Time) {
	var tips = []string{}
	for _, tip := range tangle.Tips {
		//if i >= 25 { break }
		tips = append(tips, convert.BytesToTrytes(tip.Hash)[:81])
	}
	c.JSON(http.StatusOK, gin.H{
		"hashes": tips,
		"duration": getDuration(t),
	})
}

func getTransactionsToApprove (request Request, c *gin.Context, t time.Time) {
	var trunk string
	var branch string
	trunkBytes := tangle.GetRandomTip()
	branchBytes := tangle.GetRandomTip()
	if trunkBytes != nil {
		trunk = convert.BytesToTrytes(trunkBytes)[:81]
	}
	if branchBytes != nil {
		branch = convert.BytesToTrytes(branchBytes)[:81]
	}
	c.JSON(http.StatusOK, gin.H{
		"trunkTransaction": trunk,
		"branchTransaction": branch,
		"duration": getDuration(t),
	})
}