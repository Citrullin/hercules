package api

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"server"
	"strings"
	"log"
	"time"
)

func addNeighbors (request Request, c *gin.Context, t time.Time) {
	if request.Uris != nil {
		added := 0
		for _, address := range request.Uris {
			log.Println("Adding neighbor: " + address)
			added += server.AddNeighbor(strings.Replace(address, "udp://", "", -1))
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
			log.Println("Removing neighbor: " + address)
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