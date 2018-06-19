package api

import (
	"net/http"
	"time"

	"../logs"
	"../server"
	"github.com/gin-gonic/gin"
)

func addNeighbors(request Request, c *gin.Context, t time.Time) {
	if request.Uris != nil {
		added := 0
		for _, address := range request.Uris {
			logs.Log.Info("Adding neighbor: ", address)
			err := server.AddNeighbor(address)
			if err == nil {
				added++
			} else {
				logs.Log.Warningf("Could not add neighbor %v", address)
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
			logs.Log.Info("Removing neighbor: ", address)
			err := server.RemoveNeighbor(address)
			if err == nil {
				removed++
			} else {
				logs.Log.Warningf("Could not add neighbor %v", address)
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
