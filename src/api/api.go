package api

import (
	"github.com/gin-gonic/gin"
	"log"
	"net/http"
)

type Request struct {
	Command string
	Uris    []string
}

var api *gin.Engine

func Start (address string) {
	// TODO: allow password protection for remote access
	// TODO: allow certain command to be accessed locally only
	api = gin.Default()
	// TODO: make duration work on API
	api.POST("/", func(c *gin.Context) {
		var request Request
		if err := c.ShouldBindJSON(&request); err == nil {
			if request.Command == "addNeighbors" {
				addNeighbors(request, c)
			} else if request.Command == "removeNeighbors" {
				removeNeighbors(request, c)
			} else if request.Command == "getNeighbors" {
				getNeighbors(request, c)
			} else if request.Command == "getNodeInfo" {
				// TODO: add missing fields to getInfo API
				c.JSON(http.StatusOK, gin.H{
					"appName": "CarrIOTA Nelson Go",
					"appVersion": "0.0.1",
					"duration": 0,
				})
			}
			// TODO: Add missing APIs
			// TODO: catch-all for unknown commands
		} else {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "No command provided",
			})
		}
	})
	api.Run(address)
	log.Println("API running on " + address)
}
