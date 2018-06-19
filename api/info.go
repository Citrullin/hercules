package api

import (
	"net/http"
	"time"
	"runtime"
	"github.com/gin-gonic/gin"
	"gitlab.com/semkodev/hercules/convert"
	"gitlab.com/semkodev/hercules/tangle"
	"gitlab.com/semkodev/hercules/server"
	"gitlab.com/semkodev/hercules/snapshot"
)

func getNodeInfo (request Request, c *gin.Context, t time.Time) {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	server.NeighborsLock.RLock()
	defer server.NeighborsLock.RUnlock()

	milestone := convert.BytesToTrytes(tangle.LatestMilestone.TX.Hash)[:81]
	index := tangle.LatestMilestone.Index
	solid := dummyHash
	sindex := 500000
	if snapshot.IsSynchronized() {
		solid = milestone
		sindex = index
	}
	c.JSON(http.StatusOK, gin.H{
		"appName": "CarrIOTA Hercules Go",
		"appVersion": "0.1.0",
		"availableProcessors": runtime.NumCPU(),
		"currentRoutines": runtime.NumGoroutine(),
		"allocatedMemory": stats.Sys,
		"latestMilestone": convert.BytesToTrytes(tangle.LatestMilestone.TX.Hash)[:81],
		"latestMilestoneIndex": tangle.LatestMilestone.Index,
		"latestSolidSubtangleMilestone": solid,
		"latestSolidSubtangleMilestoneIndex": sindex,
		"neighbors": len(server.Neighbors),
		"currentSnapshotTimestamp": snapshot.CurrentTimestamp,
		"isSynchronized": snapshot.IsSynchronized(),
		"tips": len(tangle.Tips),
		"time": time.Now().Unix(),
		"duration": getDuration(t),
	})
}
