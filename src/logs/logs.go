package logs

import (
	"github.com/op/go-logging"
	"os"
	"github.com/spf13/viper"
)

var LOG_FORMAT = "%{color}[%{level:.4s}] %{time:15:04:05.000000} %{id:06x} [%{longpkg}] %{longfunc} -> %{color:reset}%{message}"
var Log = logging.MustGetLogger("hercules")

func Setup() {
	backend1 := logging.NewLogBackend(os.Stdout, "", 0)
	logging.SetFormatter(logging.MustStringFormatter(LOG_FORMAT))
	logging.SetBackend(backend1)
}

func SetConfig(config *viper.Viper) {
	level, err := logging.LogLevel(config.GetString("level"))
	if err == nil {
		logging.SetLevel(level, "hercules")
	} else {
		Log.Warningf("Could not set log level to %v: %v", config.GetString("level"), err)
		Log.Warning("Using default log level")
	}
}