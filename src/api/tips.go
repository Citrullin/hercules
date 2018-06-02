package api

import (
	"github.com/gin-gonic/gin"
	"convert"
	"net/http"
	"tangle"
)

func getTips (request Request, c *gin.Context) {
	var tips []string
	for _, tip := range tangle.Tips {
		//if i >= 25 { break }
		tips = append(tips, convert.BytesToTrytes(tip.Hash)[:81])
	}
	c.JSON(http.StatusOK, gin.H{
		"hashes": tips,
		"duration": 0,
	})
}

func getTransactionsToApprove (request Request, c *gin.Context) {
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
		"duration": 0,
	})
}