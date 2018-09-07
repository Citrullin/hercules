package config

import (
	"encoding/json"
	"os"
	"strings"

	"../logs"
	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var (
	AppConfig = viper.New()
)

/*
PRECEDENCE (Higher number overrides the others):
1. default
2. key/value store
3. config
4. env
5. flag
6. explicit call to Set
*/
func Start() {
	// 1. Set defaults
	//config.SetDefault("test", 0)

	// 2. Get command line arguments
	flag.Bool("debug", false, "Run hercules when debugging the source-code")

	flag.Bool("light", false, "Whether working on a low-memory, low CPU device. Try to optimize accordingly.")

	declareApiConfigs()
	declareLogConfigs()

	flag.String("database.type", "badger", "Type of the database")
	flag.String("database.path", "data", "Path to the database directory")

	flag.String("snapshots.path", "data", "Path to the snapshots directory")
	flag.String("snapshots.filename", "", "If set, the snapshots will be saved using this name, "+
		"otherwise <timestamp>.snap wil be used")
	flag.String("snapshots.loadFile", "", "Path to a snapshot file to load")
	flag.String("snapshots.loadIRIFile", "", "Path to an IRI snapshot file to load")
	flag.String("snapshots.loadIRISpentFile", "", "Path to an IRI spent snapshot file to load")
	flag.Int("snapshots.loadIRITimestamp", 0, "Timestamp for which to load the given IRI snapshot files.")
	flag.Int("snapshots.interval", 0, "Interval in hours to automatically make the snapshots. 0 = off")
	// Lesser period increases the probability that some addresses will not be consistent with the global state.
	// If your node can handle more, we suggest keeping several days or a week worth of data!
	flag.Int("snapshots.period", 168, "How many hours of tangle data to keep after the snapshot. Minimum is 12. Default = 168 (one week)")
	flag.Bool("snapshots.enableapi", true, "Enable snapshot api commands: "+
		"makeSnapshot, getSnapshotsInfo")
	flag.Bool("snapshots.keep", false, "Whether to keep transactions past the horizon after making a snapshot.")

	flag.IntP("node.port", "u", 14600, "UDP Node port")
	flag.StringSliceP("node.neighbors", "n", nil, "Initial Node neighbors")

	AppConfig.BindPFlags(flag.CommandLine)

	var configPath = flag.StringP("config", "c", "hercules.config.json", "Config file path")
	flag.Parse()

	// 3. Bind environment vars
	replacer := strings.NewReplacer(".", "_")
	AppConfig.SetEnvPrefix("HERCULES")
	AppConfig.SetEnvKeyReplacer(replacer)
	AppConfig.AutomaticEnv()

	// 3. Load config
	if len(*configPath) > 0 {
		_, err := os.Stat(*configPath)
		if !flag.CommandLine.Changed("config") && os.IsNotExist(err) {
			// Standard config file not found => skip
			logs.Log.Info("Standard config file not found. Loading default settings.")
		} else {
			logs.Log.Infof("Loading config from: %s", *configPath)
			AppConfig.SetConfigFile(*configPath)
			err := AppConfig.ReadInConfig()
			if err != nil {
				logs.Log.Fatalf("Config could not be loaded from: %s (%s)", *configPath, err)
			}
		}
	}

	// 4. Check config for validity
	/**/
	snapshotPeriod := AppConfig.GetInt("snapshots.period")
	snapshotInterval := AppConfig.GetInt("snapshots.interval")
	if snapshotInterval > 0 && snapshotPeriod < 12 {
		logs.Log.Fatalf("The given snapshot period of %v hours is too short! "+
			"At least 12 hours currently required to be kept.", snapshotPeriod)
	}
	/**/

	logs.SetConfig(AppConfig)

	cfg, _ := json.MarshalIndent(AppConfig.AllSettings(), "", "  ")
	logs.Log.Debugf("Following settings loaded: \n %+v", string(cfg))
}

func declareApiConfigs() {
	flag.String("api.auth.username", "", "API Access Username")
	flag.String("api.auth.password", "", "API Access Password")

	flag.Bool("api.cors.setAllowOriginToAll", true, "Defines if 'Access-Control-Allow-Origin' is set to '*'")

	flag.Bool("api.http.useHttp", true, "Defines if the API will serve using HTTP protocol")
	flag.StringP("api.http.host", "h", "0.0.0.0", "HTTP API Host")
	flag.IntP("api.http.port", "p", 14265, "HTTP API Port")

	flag.Bool("api.https.useHttps", false, "Defines if the API will serve using HTTPS protocol")
	flag.String("api.https.host", "0.0.0.0", "HTTPS API Host")
	flag.Int("api.https.port", 14266, "HTTPS API Port")
	flag.String("api.https.certificatePath", "cert.pem", "Path to TLS certificate (non-encrypted)")
	flag.String("api.https.privateKeyPath", "key.pem", "Path to private key used to isse the TLS certificate (non-encrypted)")

	flag.StringSlice("api.limitRemoteAccess", nil, "Limit access to these commands from remote")

	flag.Int("api.pow.maxMinWeightMagnitude", 14, "Maximum Min-Weight-Magnitude (Difficulty for PoW)")
	flag.Int("api.pow.maxTransactions", 10000, "Maximum number of Transactions in Bundle (for PoW)")
	flag.Bool("api.pow.usePowSrv", false, "Use PowSrv (e.g. FPGA PiDiver) for PoW")
	flag.String("api.pow.powSrvPath", "/tmp/powSrv.sock", "Unix socket path of PowSrv")
}

func declareLogConfigs() {
	flag.Bool("log.hello", true, "Show welcome banner")
	flag.String("log.level", "INFO", "DEBUG, INFO, NOTICE, WARNING, ERROR or CRITICAL")

	flag.Bool("log.useRollingLogFile", false, "Enable save log messages to rolling log files")
	flag.String("log.logFile", "hercules.log", "Path to file where log files are saved")
	flag.Int32("log.maxLogFileSize", 10, "Maximum size in megabytes for log files. Default is 10MB")
	flag.Int32("log.maxLogFilesToKeep", 1, "Maximum amount of log files to keep when a new file is created. Default is 1 file")

	flag.String("log.criticalErrorsLogFile", "herculesCriticalErrors.log", "Path to file where critical error messages are saved")
}
