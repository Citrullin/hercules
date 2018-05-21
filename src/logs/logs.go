package logs

import (
	"github.com/op/go-logging"
	"os"
	"github.com/spf13/viper"
)

var Log = logging.MustGetLogger("hercules")
var log_format = "%{color}[%{level:.4s}] %{time:15:04:05.000000} %{id:06x} [%{longpkg}] %{longfunc} -> %{color:reset}%{message}"

func Setup() {
	backend1 := logging.NewLogBackend(os.Stdout, "", 0)
	logging.SetFormatter(logging.MustStringFormatter(log_format))
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