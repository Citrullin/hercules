package api

import (
	"net/http"
	"strings"
	"time"

	"../logs"
	"../server"
	"github.com/gin-gonic/gin"
)

func init() {
	addAPICall("addNeighbors", addNeighbors)
	addAPICall("removeNeighbors", removeNeighbors)
	addAPICall("getNeighbors", getNeighbors)
}

func addNeighbors(request Request, c *gin.Context, t time.Time) {
	if request.Uris != nil {
		added := 0
		for _, address := range request.Uris {
			address = strings.TrimPrefix(address, " ")
			address = strings.TrimSuffix(address, " ")
			logs.Log.Infof("Adding neighbor: '%v'", address)
			err := server.AddNeighbor(address)
			if err == nil {
				added++
			} else {
				logs.Log.Warningf("Could not add neighbor '%v' (%v)", address, err)
			}
		}
		c.JSON(http.StatusOK, gin.H{
			"addedNeighbors": added,
			"duration":       getDuration(t),
		})
	}
}

func removeNeighbors(request Request, c *gin.Context, t time.Time) {
	if request.Uris != nil {
		removed := 0
		for _, address := range request.Uris {
			address = strings.TrimPrefix(address, " ")
			address = strings.TrimSuffix(address, " ")
			logs.Log.Infof("Removing neighbor: '%v'", address)
			err := server.RemoveNeighbor(address)
			if err == nil {
				removed++
			} else {
				logs.Log.Warningf("Could not remove neighbor '%v' (%v)", address, err)
			}
		}
		c.JSON(http.StatusOK, gin.H{
			"removedNeighbors": removed,
			"duration":         getDuration(t),
		})
	}
}

func getNeighbors(request Request, c *gin.Context, t time.Time) {
	server.NeighborsLock.RLock()
	defer server.NeighborsLock.RUnlock()

	var neighbors []interface{}
	for _, neighbor := range server.Neighbors {
		neighbors = append(neighbors, gin.H{
			"address":                     neighbor.Addr,
			"numberOfAllTransactions":     neighbor.Incoming,
			"numberOfInvalidTransactions": neighbor.Invalid,
			"numberOfNewTransactions":     neighbor.New,
			"connectionType":              neighbor.ConnectionType})
	}

	if neighbors == nil {
		neighbors = make([]interface{}, 0)
	}

	c.JSON(http.StatusOK, gin.H{
		"neighbors": neighbors,
		"duration":  getDuration(t),
	})
}
