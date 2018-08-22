package main

import (
	"encoding/json"
	"os"
	"os/signal"
	"strings"
	"time"

	"./api"
	"./db"
	"./logs"
	"./server"
	"./snapshot"
	"./tangle"
	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var config *viper.Viper

func init() {
	logs.Setup()
	config = loadConfig()
	logs.SetConfig(config)

	cfg, _ := json.MarshalIndent(config.AllSettings(), "", "  ")
	logs.Log.Debugf("Following settings loaded: \n %+v", string(cfg))
}

func main() {
	Hello()
	time.Sleep(time.Duration(500) * time.Millisecond)
	StartHercules()
}

func StartHercules() {
	logs.Log.Info("Starting Hercules. Please wait...")
	db.Load(config)
	srv := server.Create(config)

	snapshot.Start(config)
	tangle.Start(srv, config)
	server.Start()
	api.Start(config)

	go db.StartPeriodicDatabaseCleanup()

	ch := make(chan os.Signal, 10)
	signal.Notify(ch, os.Interrupt)
	signal.Notify(ch, os.Kill)
	for range ch {
		// Clean exit
		logs.Log.Info("Hercules is shutting down. Please wait...")
		go func() {
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
	//config.SetDefault("test", 0)

	// 2. Get command line arguments
	flag.Bool("light", false, "Whether working on a low-memory, low CPU device. Try to optimize accordingly.")

	declareApiConfigs()
	declareLogConfigs()

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

	config.BindPFlags(flag.CommandLine)

	var configPath = flag.StringP("config", "c", "hercules.config.json", "Config file path")
	flag.Parse()

	// 3. Bind environment vars
	replacer := strings.NewReplacer(".", "_")
	config.SetEnvPrefix("HERCULES")
	config.SetEnvKeyReplacer(replacer)
	config.AutomaticEnv()

	// 3. Load config
	if len(*configPath) > 0 {
		_, err := os.Stat(*configPath)
		if !flag.CommandLine.Changed("config") && os.IsNotExist(err) {
			// Standard config file not found => skip
			logs.Log.Info("Standard config file not found. Loading default settings.")
		} else {
			logs.Log.Infof("Loading config from: %s", *configPath)
			config.SetConfigFile(*configPath)
			err := config.ReadInConfig()
			if err != nil {
				logs.Log.Fatalf("Config could not be loaded from: %s (%s)", *configPath, err)
			}
		}
	}

	// 4. Check config for validity
	/**/
	snapshotPeriod := config.GetInt("snapshots.period")
	snapshotInterval := config.GetInt("snapshots.interval")
	if snapshotInterval > 0 && snapshotPeriod < 12 {
		logs.Log.Fatalf("The given snapshot period of %v hours is too short! "+
			"At least 12 hours currently required to be kept.", snapshotPeriod)
	}
	/**/

	return config
}

func declareApiConfigs() {
	flag.Bool("api.debug", false, "Whether to log api access")

	flag.String("api.auth.username", "", "API Access Username")
	flag.String("api.auth.password", "", "API Access Password")

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

func Hello() {
	if !config.GetBool("log.hello") {
		return
	}

	logs.Log.Info(`


                                                                                    *..((/@%,%/#                                                               
                                                                                       .  .&%.%**    .                                                         
                                                                                           #%.  .@@(,                                                          
                                                                                              .*@@@@@@%                                                        
                                                                                         **  .@ @@@@&@%                                                        
                                                                                         .   @@*&@@@%                                                        
                                                                                             @(@@@@ .@                                                         
                                                                                            .(  ,                                                              
                                                                                  ,        **(@    *@                                                          
                                                                                     * %&  .      @@@.                                                         
                                                          %@@@@@@#,#% *                          .,(%                                                          
                                                       @@*@@@@@%@@@@@ .                  .   %@@#.                                                             
                                                     *@&@@@@@@@ @@@@%,.        ,.                                                                              
                                              ,&@@@#@/*,*%@@*@,@@%.                               %*/                                                          
                                            ,    ##@@@@@@@*#@%@                                    ##                                                          
                                               ,#@@@@@@@@@@@.*@  *@@%                                                                                          
                                             .**@@@@@@@@@@@@@ %@%@@@@@@@@.,                    .    ./@,                                                       
                                             *  &@@@@@@@@@@@@,*@%@@@@@@@@@@@#                   @&@.*(@@@@                                                     
                                          . (   ,/@@@@@@@@@@@, @&@@@@@@@@@@@@@      *&*#        @@@@@%@@@@@                                                    
                                                   ,(@(#%@@@@* %@*@@@@@@@@@@@@%         (@@@%@@@@@@@@@@@@@@@                                                   
                                                          .@@%*,@,#@@@@@@@@@@@&          #%@&@@@@.%@@@@@@@@@#                                                  
                                          (                 %   ./.%@(&@&@%.          .*&&@@@@(      &@@@@@                                                  
                                         .#                       ,                               *      .,%@                                                  
                                          ,                                    ..(@@@%.                                                                        
                                         ,%                                 ..    #*&@@@@#.                                                                    
                     ,                    ...    .                                                     ,%@  *                                                  
                                             (@.                              .,  *,(                  %@@@@@.                                                 
                                           /@@  .                                                      /&%@@@/                                                 
                      ,                        /                                                       #@%@@@,                                                 
                                            .                                                          @@@@@@@%                                                
                                         .&                                                             #@@@@@%                                                
                                         * *                                                  .          %@@@@%                                                
                                       * .@.                                                              &@@@@                                                
                                       ,@@@@%.                                                             #&/@,                                               
                                  /    @@@@@@@#                                                ..           ,@@@.                                              
                          .           .@@@@##@@,                                             @.            (@@@@@                                              
              ,                        @@@@@ @@.                                                          @@@@@@@                                              
                                      ,&&@@@@*@                                                 .    .%@@@@@@@@@@.                                             
                                     *(&@@@@@*,                                     .           .        @@@@@@@@,                                             
                                       &@&@@@%                                                 *#        %@@@@@/@.      .                                      
                                      &@@@@@@                                               .         /((,@@@@@%%                                              
                     .               (& (@*                                                               @@@@@@@%.                                            
	`)
	logs.Log.Info("db   db d88888b d8888b.  .o88b. db    db db      d88888b .d8888.")
	logs.Log.Info("88   88 88'     88  `8D d8P  Y8 88    88 88      88'     88'  YP")
	logs.Log.Info("88ooo88 88ooooo 88oobY' 8P      88    88 88      88ooooo `8bo.")
	logs.Log.Info("88~~~88 88~~~~~ 88`8b   8b      88    88 88      88~~~~~   `Y8b.")
	logs.Log.Info("88   88 88.     88 `88. Y8b  d8 88b  d88 88booo. 88.     db   8D")
	logs.Log.Info("YP   YP Y88888P 88   YD  `Y88P' ~Y8888P' Y88888P Y88888P `8888Y'")
}
