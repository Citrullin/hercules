package api

import (
	"net/http"
	"runtime"
	"time"

	"../convert"
	"../server"
	"../snapshot"
	"../tangle"
	"github.com/gin-gonic/gin"
)

var utcLocation, _ = time.LoadLocation("UTC")

func init() {
	addAPICall("getNodeInfo", getNodeInfo)
}

func getNodeInfo(request Request, c *gin.Context, t time.Time) {
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
		"appName":                            "CarrIOTA Hercules Go",
		"appVersion":                         "0.1.0",
		"availableProcessors":                runtime.NumCPU(),
		"currentRoutines":                    runtime.NumGoroutine(),
		"allocatedMemory":                    stats.Sys,
		"latestMilestone":                    milestone,
		"latestMilestoneIndex":               tangle.LatestMilestone.Index,
		"latestSolidSubtangleMilestone":      solid,
		"latestSolidSubtangleMilestoneIndex": sindex,
		"neighbors":                          len(server.Neighbors),
		"currentSnapshotTimestamp":           snapshot.CurrentTimestamp,
		"currentSnapshotTimeHumanReadable":   getHumanReadableTime(snapshot.CurrentTimestamp),
		"isSynchronized":                     snapshot.IsSynchronized(),
		"tips":                               len(tangle.Tips),
		"time":                               time.Now().Unix(),
		"duration":                           getDuration(t),
	})
}

func getHumanReadableTime(timestamp int) string {
	if timestamp <= 0 {
		return ""
	} else {
		unitxTime := time.Unix(int64(timestamp), 0)
		return unitxTime.In(utcLocation).Format(time.RFC822)
	}
}
