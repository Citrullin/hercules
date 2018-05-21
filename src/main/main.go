package main

import (
	"server"
	flag "github.com/spf13/pflag"
	"strings"
	"time"
	"tangle"
	"db"
	"os"
	"os/signal"
	"api"
	"logs"
	"github.com/spf13/viper"
	"encoding/json"
)

var config *viper.Viper

func init() {
	logs.Setup()
	config = loadConfig()
	logs.SetConfig(config.Sub("log"))

	cfg, _ := json.MarshalIndent(config.AllSettings(), "", "  ")
	logs.Log.Debugf("Following settings loaded: \n %+v", string(cfg))
}

func main () {
	StartHercules()
}

func StartHercules () {
	logs.Log.Info("Starting Hercules. Please wait...")
	db.Load(&db.DatabaseConfig{config.GetString("database.path"), 10})
	srv := server.Create(config.Sub("node"))
	tangle.Start(srv)
	api.Start(config.Sub("api"))

	ch := make(chan os.Signal, 10)
	signal.Notify(ch, os.Interrupt)
	signal.Notify(ch, os.Kill)
	for range ch {
		// Clean exit
		logs.Log.Info("Hercules is shutting down. Please wait...")
		go func () {
			time.Sleep(time.Duration(5000) * time.Millisecond)
			logs.Log.Info("Bye!")
			os.Exit(0)
		}()
		go api.End()
		go server.End()
		db.End()
	}
}

/*
PRECEDENCE (Higher number overrides the others):
1. default
2. key/value store
3. config
4. env
5. flag
6. explicit call to Set
 */
func loadConfig() *viper.Viper {
	// Setup Viper
	var config = viper.New()

	// 1. Set defaults
	config.SetDefault("test", 0)

	// 2. Get command line arguments
	flag.StringP("config", "c", "", "Config path")

	flag.IntP("api.port", "p", 14265, "API Port")
	flag.String("api.auth.username", "", "API Access Username")
	flag.String("api.auth.password", "", "API Access Password")
	flag.Bool("api.debug", false, "Whether to log api access")
	flag.StringSlice("api.limitRemoteAccess",nil, "Limit access to these commands from remote")

	flag.String("log.level", "data", "DEBUG, INFO, NOTICE, WARNING, ERROR or CRITICAL")

	flag.String("database.path", "data", "Path to the database directory")

	flag.String("snapshots.path", "data", "Path to the snapshots directory")

	flag.IntP("node.port", "u", 13600, "UDP Node port")
	flag.StringSliceP("node.neighbors","n", nil, "Initial Node neighbors")

	flag.Parse()
	config.BindPFlags(flag.CommandLine)

	// 3. Bind environment vars
	replacer := strings.NewReplacer(".", "_")
	config.SetEnvPrefix("HERCULES")
	config.SetEnvKeyReplacer(replacer)
	config.AutomaticEnv()

	// 3. Load config
	var configPath = config.GetString("config")
	if len(configPath) > 0 {
		logs.Log.Infof("Loading config from: %s", configPath)
		config.SetConfigFile(configPath)
		err := config.ReadInConfig()
		if err != nil {
			logs.Log.Fatalf("Config could not be loaded from: %s", configPath)
		}
	}

	return config
}