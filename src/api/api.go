package api

import (
	"context"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
	"logs"
	"github.com/spf13/viper"
)

type Request struct {
	Command string
	Uris    []string
}

var api *gin.Engine
var srv *http.Server
var config *viper.Viper

func Start (apiConfig *viper.Viper) {
	config = apiConfig
	if !config.GetBool("debug") {
		gin.SetMode(gin.ReleaseMode)
	}
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
	srv = &http.Server{
		Addr:    ":" + config.GetString("port"),
		Handler: api,
	}
	go func() {
		// service connections
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logs.Log.Fatal("API Server Error", err)
		}
	}()
}

func End () {
	if srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			logs.Log.Fatal("API Server Shutdown Error:", err)
		}
		logs.Log.Info("API Server exiting...")
	}
}