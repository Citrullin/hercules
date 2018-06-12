package api

import (
	"time"
	"runtime"
	"net/http"
    "io/ioutil"
	"strings"
	"strconv"
	"os"
	"path"
	"crypto/md5"
	"io"
	"encoding/hex"
	"fmt"
	"github.com/gin-gonic/gin"
	"gitlab.com/semkodev/hercules/logs"
	"gitlab.com/semkodev/hercules/snapshot"
)

func enableSnapshotApi(api *gin.Engine) {
	api.POST("/snapshots", func(c *gin.Context) {
		t := time.Now()
		var request Request
		if err := c.ShouldBindJSON(&request); err == nil {
			if triesToAccessLimited(request.Command, c) {
				logs.Log.Warningf("Denying limited command request %v from remote %v",
					request.Command, c.Request.RemoteAddr)
				ReplyError("Limited remote command access", c)
				return
			}
			if request.Command == "getSnapshotsInfo" {
				getSnapshotsInfo(request, c, t)
			} else if request.Command == "makeSnapshot" {
				makeSnapshot(request, c, t)
			} else {
				logs.Log.Error("Unknown command", request.Command)
				ReplyError("No known command provided", c)
			}
		} else {
			logs.Log.Error("ERROR request", err)
			ReplyError("Wrongly formed JSON", c)
		}
	})

	dir := config.GetString("snapshots.path")
	api.Static("/snapshots", dir)
}

func getSnapshotsInfo (request Request, c *gin.Context, t time.Time) {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)

	var timestamps []map[string]interface{}

	dir := config.GetString("snapshots.path")
	files, err := ioutil.ReadDir(dir)
	if err == nil {
		for _, f := range files {
			name := f.Name()
			tokens := strings.Split(name, ".")
			if len(tokens) == 2 && tokens[1] == "snap" {
				timestamp, err := strconv.ParseInt(tokens[0], 10, 64)
				if err == nil {
					checksum, err := fileHash(path.Join(dir, name))
					if err == nil {
						timestamps = append(timestamps, gin.H{
							"timestamp": timestamp,
							"path": "/snapshots/" + name,
							"checksum": checksum,
						})
					}
				}
			}
		}
	}

	if timestamps == nil {
		timestamps = make([]map[string]interface{}, 0)
	}

	c.JSON(http.StatusOK, gin.H{
		"currentSnapshotTimestamp": snapshot.CurrentTimestamp,
		"isSynchronized": snapshot.IsSynchronized(),
		"unfinishedSnapshotTimestamp": snapshot.GetSnapshotLock(nil),
		"inProgress": snapshot.InProgress,
		"snapshots": timestamps,
		"time": time.Now().Unix(),
		"duration": getDuration(t),
	})
}

func makeSnapshot(request Request, c *gin.Context, t time.Time) {
	if request.Timestamp < 1525017600 || request.Timestamp > int(time.Now().Unix()) {
		ReplyError("Wrong UNIX timestamp provided", c)
		return
	}

	if snapshot.InProgress {
		ReplyError("A snapshot is currently in progress", c)
		return
	}

	current := snapshot.GetSnapshotLock(nil)
	if current > 0 && current != request.Timestamp {
		ReplyError(
			fmt.Sprintf("A snapshot is currently pending. Finish it first: %v", current),
			c)
		return
	}

	if !snapshot.IsSynchronized() {
		ReplyError("The tangle not fully synchronized. Cannot snapshot in this state.", c)
		return
	}

	if !snapshot.CanSnapshot(request.Timestamp) {
		ReplyError("Pending confirmations behind the snapshot horizon. Cannot snapshot in this state.", c)
		return
	}

	go snapshot.MakeSnapshot(request.Timestamp)
	c.JSON(http.StatusOK, gin.H{
		"time": time.Now().Unix(),
		"duration": getDuration(t),
	})
}

func fileHash(filePath string) (string, error) {
	//Initialize variable returnMD5String now in case an error has to be returned
	var returnMD5String string

	//Open the passed argument and check for any error
	file, err := os.Open(filePath)
	if err != nil {
		return returnMD5String, err
	}

	//Tell the program to call the following function when the current function returns
	defer file.Close()

	//Open a new hash interface to write to
	hash := md5.New()

	//Copy the file in the hash interface and check for any error
	if _, err := io.Copy(hash, file); err != nil {
		return returnMD5String, err
	}

	//Get the 16 bytes hash
	hashInBytes := hash.Sum(nil)[:16]

	//Convert the bytes to a string
	returnMD5String = hex.EncodeToString(hashInBytes)

	return returnMD5String, nil

}