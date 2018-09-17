package api

import (
	"fmt"
	"net/http"
	"time"

	"../convert"
	"../tangle"
	"github.com/gin-gonic/gin"
)

func init() {
	addAPICall("getTips", getTips, mainAPICalls)
	addAPICall("getTransactionsToApprove", getTransactionsToApprove, mainAPICalls)
}

func getTips(request Request, c *gin.Context, ts time.Time) {
	var tips = []string{}
	tangle.TipsLock.RLock()
	for hash := range tangle.Tips {
		//if i >= 25 { break }
		tips = append(tips, convert.BytesToTrytes([]byte(hash))[:81])
	}
	tangle.TipsLock.RUnlock()
	c.JSON(http.StatusOK, gin.H{
		"hashes":   tips,
		"duration": getDuration(ts),
	})
}

func getTransactionsToApprove(request Request, c *gin.Context, ts time.Time) {
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

	tips, err := tangle.GetTXToApprove(reference, request.Depth)
	if err != nil {
		replyError(fmt.Sprint(err), c)
		return
	}

	if tips == nil || len(tips) < 2 {
		replyError("Could not get transactions to approve", c)
		return
	}

	trunk := convert.BytesToTrytes(tips[0])
	branch := convert.BytesToTrytes(tips[1])

	if len(trunk) < 81 || len(branch) < 81 {
		replyError("Could not get transactions to approve", c)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"trunkTransaction":  trunk[:81],
		"branchTransaction": branch[:81],
		"duration":          getDuration(ts),
	})
}
