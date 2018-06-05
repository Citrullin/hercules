package api

import (
	"net/http"
	"github.com/gin-gonic/gin"
	"time"
	"runtime"
	"convert"
	"tangle"
	"server"
	"snapshot"
)

func getNodeInfo (request Request, c *gin.Context, t time.Time) {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)

	c.JSON(http.StatusOK, gin.H{
		"appName": "CarrIOTA Hercules Go",
		"appVersion": "0.1.0",
		"availableProcessors": runtime.NumCPU(),
		"currentRoutines": runtime.NumGoroutine(),
		"allocatedMemory": stats.Sys,
		"latestMilestone": convert.BytesToTrytes(tangle.LatestMilestone.TX.Hash)[:81],
		"latestMilestoneIndex": tangle.LatestMilestone.Index,
		"neighbors": len(server.Neighbors),
		"currentSnapshotTimestamp": snapshot.CurrentTimestamp,
		"isSynchronized": snapshot.IsSynchronized(),
		"tips": len(tangle.Tips),
		"time": time.Now().Unix(),
		"duration": getDuration(t),
	})
}
