package api

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"server"
	"strings"
	"log"
)

func addNeighbors (request Request, c *gin.Context) {
	if request.Uris != nil {
		added := 0
		for _, address := range request.Uris {
			log.Println("Adding neighbor: " + address)
			added += server.AddNeighbor(strings.Replace(address, "udp://", "", -1))
		}
		c.JSON(http.StatusOK, gin.H{
			"addedNeighbors": added,
			"duration": 0,
		})
	}
}

func removeNeighbors (request Request, c *gin.Context) {
	if request.Uris != nil {
		removed := 0
		for _, address := range request.Uris {
			log.Println("Removing neighbor: " + address)
			removed += server.RemoveNeighbor(strings.Replace(address, "udp://", "", -1))
		}
		c.JSON(http.StatusOK, gin.H{
			"removedNeighbors": removed,
			"duration":       0,
		})
	}
}

func getNeighbors (request Request, c *gin.Context) {
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
	c.JSON(http.StatusOK, gin.H{
		"neighbors": neighbors,
		"duration":       0,
	})
}