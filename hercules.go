package main

import (
	"encoding/json"
	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"
	"gitlab.com/semkodev/hercules/api"
	"gitlab.com/semkodev/hercules/db"
	"gitlab.com/semkodev/hercules/logs"
	"gitlab.com/semkodev/hercules/server"
	"gitlab.com/semkodev/hercules/snapshot"
	"gitlab.com/semkodev/hercules/tangle"
	"os"
	"os/signal"
	"strings"
	"time"
)

var config *viper.Viper

func init() {
	logs.Setup()
	config = loadConfig()
	logs.SetConfig(config.Sub("log"))

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

	//snapshot.LoadAddressBytes("snapshots/1528630000.snap")
	snapshot.Start(config)
	tangle.Start(srv, config)
	server.Start()
	api.Start(config)

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
	config.SetDefault("test", 0)

	// 2. Get command line arguments
	flag.StringP("config", "c", "", "Config path")

	flag.Bool("light", false, "Whether working on a low-memory, low CPU device. "+
		"Try to optimize accordingly.")

	flag.IntP("api.port", "p", 14265, "API Port")
	flag.StringP("api.host", "h", "0.0.0.0", "API Host")
	flag.String("api.auth.username", "", "API Access Username")
	flag.String("api.auth.password", "", "API Access Password")
	flag.Bool("api.debug", false, "Whether to log api access")
	flag.StringSlice("api.limitRemoteAccess", nil, "Limit access to these commands from remote")

	flag.String("log.level", "data", "DEBUG, INFO, NOTICE, WARNING, ERROR or CRITICAL")

	flag.String("database.path", "data", "Path to the database directory")

	flag.String("snapshots.path", "data", "Path to the snapshots directory")
	flag.Int("snapshots.interval", 0, "Interval in hours to automatically make the snapshots. Minimum 3.")
	flag.Int("snapshots.period", 24, "How many hours of tangle data to keep after the snapshot. Minimum: 6.")
	flag.Bool("snapshots.enableapi", true, "Enable snapshot api commands: "+
		"makeSnapshot, getSnapshotsInfo")

	flag.IntP("node.port", "u", 13600, "UDP Node port")
	flag.StringSliceP("node.neighbors", "n", nil, "Initial Node neighbors")

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

	// 4. Check config for validity
	snapshotPeriod := config.GetInt("snapshots.period")
	snapshotInterval := config.GetInt("snapshots.interval")
	if snapshotInterval > 0 && snapshotPeriod < 6 {
		logs.Log.Fatalf("The given snapshot period of %v hours is too short! "+
			"At least 6 hours currently required to be kept.", snapshotPeriod)
	}

	return config
}

func Hello() {
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