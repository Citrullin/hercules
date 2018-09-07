package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"../config"
	"../logs"
	"github.com/gin-gonic/gin"
)

type Request struct {
	Command      string
	Hashes       []string
	Uris         []string
	Addresses    []string
	Bundles      []string
	Tags         []string
	Approvees    []string
	Transactions []string
	Trytes       []string
	Reference    string
	Depth        int
	Timestamp    int64
	Filename     string
	// for attachToTangle
	TrunkTransaction   string
	BranchTransaction  string
	MinWeightMagnitude int
}

var api *gin.Engine
var srv *http.Server
var limitAccess []string
var authEnabled = false
var dummyHash = strings.Repeat("9", 81)
var apiCalls = make(map[string]func(request Request, c *gin.Context, t time.Time))

// TODO: Add attach/interrupt attaching api
// TODO: limit requests, lists, etc.

func Start() {

	if !config.AppConfig.GetBool("debug") {
		gin.SetMode(gin.ReleaseMode)
	}

	configureLimitAccess()

	api = gin.Default()

	username := config.AppConfig.GetString("api.auth.username")
	password := config.AppConfig.GetString("api.auth.password")
	if len(username) > 0 && len(password) > 0 {
		api.Use(gin.BasicAuth(gin.Accounts{username: password}))
	}

	api.Use(CORSMiddleware(config.AppConfig.GetBool("api.cors.setAllowOriginToAll")))

	api.POST("/", func(c *gin.Context) {
		t := time.Now()

		var request Request
		err := c.ShouldBindJSON(&request)
		if err == nil {
			caseInsensitiveCommand := strings.ToLower(request.Command)
			if triesToAccessLimited(caseInsensitiveCommand, c) {
				logs.Log.Infof("Denying limited command request %v from remote %v", request.Command, c.Request.RemoteAddr)
				ReplyError("Limited remote command access", c)
				return
			}

			apiCall, apiCallExists := apiCalls[caseInsensitiveCommand]
			if apiCallExists {
				apiCall(request, c, t)
			} else {
				logs.Log.Error("Unknown command", request.Command)
				ReplyError("No known command provided", c)
			}

		} else {
			logs.Log.Error("ERROR request", err)
			ReplyError("Wrongly formed JSON", c)
		}
	})

	if config.AppConfig.GetBool("snapshots.enableapi") {
		enableSnapshotApi(api)
	}

	useHTTP := config.AppConfig.GetBool("api.http.useHttp")
	useHTTPS := config.AppConfig.GetBool("api.https.useHttps")

	if !useHTTP && !useHTTPS {
		logs.Log.Fatal("Either useHttp, useHttps, or both must set to true")
	}

	if useHTTP {
		go serveHttp(api)
	}

	if useHTTPS {
		go serveHttps(api)
	}
}

func CORSMiddleware(setAllowOriginToAll bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if setAllowOriginToAll {
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		}
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, X-IOTA-API-Version")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func serveHttps(api *gin.Engine) {
	serveOnAddress := config.AppConfig.GetString("api.https.host") + ":" + config.AppConfig.GetString("api.https.port")
	logs.Log.Info("API listening on HTTPS (" + serveOnAddress + ")")

	certificatePath := config.AppConfig.GetString("api.https.certificatePath")
	privateKeyPath := config.AppConfig.GetString("api.https.privateKeyPath")

	if err := http.ListenAndServeTLS(serveOnAddress, certificatePath, privateKeyPath, api); err != nil && err != http.ErrServerClosed {
		logs.Log.Fatal("API Server Error", err)
	}
}

func serveHttp(api *gin.Engine) {
	serveOnAddress := config.AppConfig.GetString("api.http.host") + ":" + config.AppConfig.GetString("api.http.port")
	logs.Log.Info("API listening on HTTP (" + serveOnAddress + ")")

	srv = &http.Server{
		Addr:    serveOnAddress,
		Handler: api,
	}

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logs.Log.Fatal("API Server Error", err)
	}
}

func End() {
	if srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := srv.Shutdown(ctx); err != nil {
			logs.Log.Fatal("API Server Shutdown Error:", err)
		}
		logs.Log.Debug("API Server exited")
		cancel()
	}
}

func ReplyError(message string, c *gin.Context) {
	c.JSON(http.StatusBadRequest, gin.H{
		"error": message,
	})
}

func getDuration(t time.Time) int32 {
	return int32(time.Now().Sub(t).Nanoseconds() / int64(time.Millisecond))
}

func configureLimitAccess() {
	localLimitAccess := config.AppConfig.GetStringSlice("api.limitRemoteAccess")

	if len(localLimitAccess) > 0 {
		for _, limitAccessEntry := range localLimitAccess {
			limitAccess = append(limitAccess, strings.ToLower(limitAccessEntry))
		}

		logs.Log.Debug("Limited remote access to:", localLimitAccess)
	}
}

func triesToAccessLimited(caseInsensitiveCommand string, c *gin.Context) bool {
	if c.Request.RemoteAddr[:9] == "127.0.0.1" {
		return false
	}
	for _, caseInsensitiveLimitAccessEntry := range limitAccess {
		if caseInsensitiveLimitAccessEntry == caseInsensitiveCommand {
			return true
		}
	}
	return false
}

func addAPICall(apiCall string, implementation func(request Request, c *gin.Context, t time.Time)) {
	caseInsensitiveAPICall := strings.ToLower(apiCall)
	apiCalls[caseInsensitiveAPICall] = implementation
}
