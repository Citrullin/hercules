package api

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"../config"
	"../logs"
	"../snapshot"
	"../utils"
	"github.com/gin-gonic/gin"
)

var snapshotAPICalls = make(map[string]APIImplementation)

func init() {
	addAPICall("getSnapshotsInfo", getSnapshotsInfo, snapshotAPICalls)
	addAPICall("getLatestSnapshotInfo", getLatestSnapshotInfo, snapshotAPICalls)
	addAPICall("makeSnapshot", makeSnapshot, snapshotAPICalls)
}

func enableSnapshotAPI(api *gin.Engine) {
	dir := config.AppConfig.GetString("snapshots.path")
	api.Static("/snapshots", dir)
}

func getSnapshotsInfo(request Request, c *gin.Context, ts time.Time) {
	const latestOnly bool = false
	response := getSnapshotsInfoResponse(latestOnly, ts)

	c.JSON(http.StatusOK, response)
}

func getLatestSnapshotInfo(request Request, c *gin.Context, ts time.Time) {
	const latestOnly bool = true
	response := getSnapshotsInfoResponse(latestOnly, ts)

	c.JSON(http.StatusOK, response)
}

func getSnapshotsInfoResponse(latestOnly bool, ts time.Time) gin.H {
	snapshotInfos := loadInfos(latestOnly)
	snapshotInfosResponseHeader, snapshotInfosResponseValue := getSnapshotInfosResponseHeaderAndValue(latestOnly, snapshotInfos)

	unfinishedSnapshotTimestamp := snapshot.GetSnapshotLock(nil)

	response := gin.H{
		"currentSnapshotTimestamp":            snapshot.CurrentTimestamp,
		"currentSnapshotTimeHumanReadable":    utils.GetHumanReadableTime(snapshot.CurrentTimestamp),
		"isSynchronized":                      snapshot.IsSynchronized(),
		"unfinishedSnapshotTimestamp":         unfinishedSnapshotTimestamp,
		"unfinishedSnapshotTimeHumanReadable": utils.GetHumanReadableTime(unfinishedSnapshotTimestamp),
		"inProgress":                          snapshot.SnapshotInProgress,
		snapshotInfosResponseHeader:           snapshotInfosResponseValue,
		"time":     time.Now().Unix(),
		"duration": getDuration(ts),
	}

	return response
}

func getSnapshotInfosResponseHeaderAndValue(latestOnly bool, snapshotInfos []map[string]interface{}) (snapshotsResponseHeader string, snapshotInfosResponseValue interface{}) {

	if latestOnly {
		snapshotsResponseHeader = "latestSnapshot"
		if len(snapshotInfos) == 1 {
			snapshotInfosResponseValue = snapshotInfos[0]
		}
	} else {
		snapshotsResponseHeader = "snapshots"
		snapshotInfosResponseValue = snapshotInfos
	}

	return
}

func loadInfos(latestOnly bool) (infos []map[string]interface{}) {
	dir := config.AppConfig.GetString("snapshots.path")
	files, err := ioutil.ReadDir(dir)
	if err == nil {

		if latestOnly {
			file := getLatestSnapshotFile(dir, files)
			info := getInfoIfValidSnapshot(dir, file)
			if info != nil {
				infos = append(infos, info)
			}
		} else {
			for _, file := range files {
				info := getInfoIfValidSnapshot(dir, file)
				if info != nil {
					infos = append(infos, info)
				}
			}

		}
	}

	if infos == nil {
		infos = make([]map[string]interface{}, 0)
	}

	return
}

func getLatestSnapshotFile(dir string, files []os.FileInfo) os.FileInfo {

	latestSnapshotFileTimestamp := int64(0)
	var latestSnapshotFile os.FileInfo

	for _, file := range files {
		fileName := file.Name()
		filePath := path.Join(dir, fileName)
		snapshotHeader, err := snapshot.LoadHeader(filePath)
		if err != nil {
			logs.Log.Errorf("Error while loading header from '%s'. Cause: %s", filePath, err)
			continue
		}

		if snapshotHeader == nil {
			continue
		}

		if snapshotHeader.Timestamp > latestSnapshotFileTimestamp {
			latestSnapshotFileTimestamp = snapshotHeader.Timestamp
			latestSnapshotFile = file
		}
	}

	return latestSnapshotFile
}

func getInfoIfValidSnapshot(dir string, file os.FileInfo) gin.H {
	if file == nil {
		return nil
	}

	fileName := file.Name()
	tokens := strings.Split(fileName, ".")
	if len(tokens) == 2 && tokens[1] == "snap" {
		timestamp, err := strconv.ParseInt(tokens[0], 10, 64)
		if err == nil {
			checksum, err := fileHash(path.Join(dir, fileName))
			if err == nil {
				return gin.H{
					"timestamp":         timestamp,
					"TimeHumanReadable": utils.GetHumanReadableTime(timestamp),
					"path":              "/snapshots/" + fileName,
					"checksum":          checksum,
				}
			}
		}
	}

	return nil
}

func makeSnapshot(request Request, c *gin.Context, ts time.Time) {
	if request.Timestamp < 1525017600 || request.Timestamp > time.Now().Unix() {
		replyError("Wrong UNIX timestamp provided", c)
		return
	}

	if snapshot.SnapshotInProgress {
		replyError("A snapshot is currently in progress", c)
		return
	}

	current := snapshot.GetSnapshotLock(nil)
	if current > 0 && current != request.Timestamp {
		replyError(
			fmt.Sprintf("A snapshot is currently pending. Finish it first: %v", current),
			c)
		return
	}

	if !snapshot.IsSynchronized() {
		replyError("The tangle not fully synchronized. Cannot snapshot in this state.", c)
		return
	}

	if !snapshot.CanSnapshot(request.Timestamp) {
		replyError("Pending confirmations behind the snapshot horizon. Cannot snapshot in this state.", c)
		return
	}

	go snapshot.MakeSnapshot(request.Timestamp, request.Filename)
	c.JSON(http.StatusOK, gin.H{
		"time":     time.Now().Unix(),
		"duration": getDuration(ts),
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
