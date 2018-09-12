package api

import (
	"net/http"
	"time"

	"../convert"
	"../logs"
	"../tangle"
	"github.com/gin-gonic/gin"
)

func init() {
	addAPICall("getTips", getTips, mainAPICalls)
	addAPICall("getTransactionsToApprove", getTransactionsToApprove, mainAPICalls)
}

func getTips(request Request, c *gin.Context, t time.Time) {
	var tips = []string{}
	tangle.TipsLock.RLock()
	for hash := range tangle.Tips {
		//if i >= 25 { break }
		tips = append(tips, convert.BytesToTrytes([]byte(hash))[:81])
	}
	tangle.TipsLock.RUnlock()
	c.JSON(http.StatusOK, gin.H{
		"hashes":   tips,
		"duration": getDuration(t),
	})
}

func getTransactionsToApprove(request Request, c *gin.Context, t time.Time) {
	if (request.Depth < tangle.MinTipselDepth) || (request.Depth > tangle.MaxTipselDepth) {
		replyError("Invalid depth input", c)
		return
	}

	var reference []byte
	if len(request.Reference) > 0 && !convert.IsTrytes(request.Reference, 81) {
		replyError("Wrong reference trytes", c)
		return
	} else if len(request.Reference) > 0 {
		reference = convert.TrytesToBytes(request.Reference)[:49]
	}

	if len(reference) < 49 {
		reference = nil
	}

	// Use it when fixed
	//tips := tangle.GetTXToApprove(reference, request.Depth)
	tips := tangle.GetRandomTXToApprove()

	if tips == nil || len(tips) < 2 {
		replyError("Could not get transactions to approve", c)
		return
	}

	logs.Log.Warningf("Tips[0]: %v", tips[0])
	logs.Log.Warningf("Tips[1]: %v", tips[1])
	trunk := convert.BytesToTrytes(tips[0])
	branch := convert.BytesToTrytes(tips[1])

	if len(trunk) < 81 || len(branch) < 81 {
		replyError("Could not get transactions to approve", c)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"trunkTransaction":  trunk[:81],
		"branchTransaction": branch[:81],
		"duration":          getDuration(t),
	})
}
