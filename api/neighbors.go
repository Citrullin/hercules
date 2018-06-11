package api

import (
	"time"
	"net/http"
	"strings"
	"github.com/gin-gonic/gin"
	"gitlab.com/semkodev/hercules.go/server"
	"gitlab.com/semkodev/hercules.go/logs"
)

func addNeighbors (request Request, c *gin.Context, t time.Time) {
	if request.Uris != nil {
		added := 0
		for _, address := range request.Uris {
			logs.Log.Info("Adding neighbor: ", address)
			err := server.AddNeighbor(strings.Replace(address, "udp://", "", -1))
			if err == nil {
				added++
			} else {
				logs.Log.Warningf("Could not add neighbor %v", address)
			}
		}
		c.JSON(http.StatusOK, gin.H{
			"addedNeighbors": added,
			"duration": getDuration(t),
		})
	}
}

func removeNeighbors (request Request, c *gin.Context, t time.Time) {
	if request.Uris != nil {
		removed := 0
		for _, address := range request.Uris {
			logs.Log.Info("Removing neighbor: ", address)
			removed += server.RemoveNeighbor(strings.Replace(address, "udp://", "", -1))
		}
		c.JSON(http.StatusOK, gin.H{
			"removedNeighbors": removed,
			"duration": getDuration(t),
		})
	}
}

func getNeighbors (request Request, c *gin.Context, t time.Time) {
	server.NeighborsLock.RLock()
	defer server.NeighborsLock.RUnlock()

	var neighbors []interface{}
	for _, neighbor := range server.Neighbors {
		neighbors = append(neighbors, gin.H{
			"address": "udp://" + neighbor.Addr,
			"numberOfAllTransactions": neighbor.Incoming,
			"numberOfInvalidTransactions": neighbor.Invalid,
			"numberOfNewTransactions": neighbor.New})
	}

	if neighbors == nil {
		neighbors = make([]interface{}, 0)
	}

	c.JSON(http.StatusOK, gin.H{
		"neighbors": neighbors,
		"duration":  getDuration(t),
	})
}