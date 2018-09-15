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

var (
	api          = gin.Default()
	dummyHash    = strings.Repeat("9", 81)
	mainAPICalls = make(map[string]APIImplementation)
	srv          *http.Server
	limitAccess  []string
)

func Start() {

	if !config.AppConfig.GetBool("debug") {
		gin.SetMode(gin.ReleaseMode)
	}

	configureLimitAccess()
	configureAPIUserAuthentication()
	configureCORSMiddleware()

	createAPIEndpoint("", mainAPICalls)

	if config.AppConfig.GetBool("snapshots.enableapi") {
		createAPIEndpoint("/snapshots", snapshotAPICalls)
		enableSnapshotAPI(api)
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

	startAttach()
}

func configureAPIUserAuthentication() {
	username := config.AppConfig.GetString("api.auth.username")
	password := config.AppConfig.GetString("api.auth.password")
	if len(username) > 0 && len(password) > 0 {
		api.Use(gin.BasicAuth(gin.Accounts{username: password}))
	}
}

func configureCORSMiddleware() {
	setAllowOriginToAll := config.AppConfig.GetBool("api.cors.setAllowOriginToAll")

	corsMiddleware := func(c *gin.Context) {
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
	api.Use(corsMiddleware)
}

func serveHttps(api *gin.Engine) {
	serveOnAddress := config.AppConfig.GetString("api.https.host") + ":" + config.AppConfig.GetString("api.https.port")
	logs.Log.Info("API listening on HTTPS (" + serveOnAddress + ")")

	certificatePath := config.AppConfig.GetString("api.https.certificatePath")
	privateKeyPath := config.AppConfig.GetString("api.https.privateKeyPath")

	if err := http.ListenAndServeTLS(serveOnAddress, certificatePath, privateKeyPath, api); err != nil && err != http.ErrServerClosed {
		logs.Log.Fatal("API server error", err)
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
		logs.Log.Fatal("API server error", err)
	}
}

func End() {
	if srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := srv.Shutdown(ctx); err != nil {
			logs.Log.Fatal("API server Shutdown Error:", err)
		}
		cancel()
		logs.Log.Debug("API server exited")
	}
}

func replyError(message string, c *gin.Context) {
	c.JSON(http.StatusBadRequest, gin.H{
		"error": message,
	})
}

func getDuration(t time.Time) int32 {
	return int32(time.Now().Sub(t).Nanoseconds() / int64(time.Millisecond))
}

type APIImplementation func(request Request, c *gin.Context, t time.Time)

func createAPIEndpoint(endpointPath string, endpointImplementation map[string]APIImplementation) {
	api.POST(endpointPath, func(c *gin.Context) {
		t := time.Now()

		var request Request
		err := c.ShouldBindJSON(&request)
		if err == nil {
			caseInsensitiveCommand := strings.ToLower(request.Command)
			if triesToAccessLimited(caseInsensitiveCommand, c) {
				logs.Log.Infof("Denying limited command request %v from remote %v", request.Command, c.Request.RemoteAddr)
				replyError("Limited remote command access", c)
				return
			}

			implementation, apiCallExists := endpointImplementation[caseInsensitiveCommand]
			if apiCallExists {
				implementation(request, c, t)
			} else {
				logs.Log.Error("Unknown command", request.Command)
				replyError("No known command provided", c)
			}

		} else {
			logs.Log.Error("ERROR request", err)
			replyError("Wrongly formed JSON", c)
		}
	})

}

func addAPICall(apiCall string, implementation APIImplementation, implementations map[string]APIImplementation) {
	caseInsensitiveAPICall := strings.ToLower(apiCall)
	implementations[caseInsensitiveAPICall] = implementation
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
