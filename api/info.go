package api

import (
	"net/http"
	"runtime"
	"time"

	"gitlab.com/semkodev/hercules/convert"
	"gitlab.com/semkodev/hercules/server"
	"gitlab.com/semkodev/hercules/snapshot"
	"gitlab.com/semkodev/hercules/tangle"
	"gitlab.com/semkodev/hercules/utils"
	"github.com/gin-gonic/gin"
)

func init() {
	addAPICall("getNodeInfo", getNodeInfo, mainAPICalls)
}

func getNodeInfo(request Request, c *gin.Context, ts time.Time) {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)

	server.NeighborsLock.RLock()
	neighborsCnt := len(server.Neighbors)
	server.NeighborsLock.RUnlock()

	tangle.TipsLock.RLock()
	tipsCnt := len(tangle.Tips)
	tangle.TipsLock.RUnlock()

	milestone := convert.BytesToTrytes(tangle.LatestMilestone.TX.Hash)[:81]
	index := tangle.LatestMilestone.Index
	solid := dummyHash
	sindex := 500000
	if snapshot.IsSynchronized() {
		solid = milestone
		sindex = index
	}
	c.JSON(http.StatusOK, gin.H{
		"appName":                            "Deviota Hercules Go",
		"appVersion":                         "0.1.0",
		"availableProcessors":                runtime.NumCPU(),
		"currentRoutines":                    runtime.NumGoroutine(),
		"allocatedMemory":                    stats.Sys,
		"latestMilestone":                    milestone,
		"latestMilestoneIndex":               tangle.LatestMilestone.Index,
		"latestSolidSubtangleMilestone":      solid,
		"latestSolidSubtangleMilestoneIndex": sindex,
		"neighbors":                          neighborsCnt,
		"currentSnapshotTimestamp":           snapshot.CurrentTimestamp,
		"currentSnapshotTimeHumanReadable":   utils.GetHumanReadableTime(snapshot.CurrentTimestamp),
		"isSynchronized":                     snapshot.IsSynchronized(),
		"tips":                               tipsCnt,
		"time":                               time.Now().Unix(),
		"duration":                           getDuration(ts),
	})
}
