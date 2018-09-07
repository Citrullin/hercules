package logs

import (
	"os"

	"github.com/op/go-logging"
	"github.com/spf13/viper"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	logFormat = "%{color}[%{level:.4s}] %{time:15:04:05.000000} %{id:06x} [%{shortpkg}] %{longfunc} -> %{color:reset}%{message}"
	Log       = logging.MustGetLogger("hercules")
	config    *viper.Viper
)

func Start() {
	logging.SetFormatter(logging.MustStringFormatter(logFormat))
}

func SetConfig(viperConfig *viper.Viper) {
	config = viperConfig
	consoleBackEnd := logging.NewLogBackend(os.Stdout, "", 0)

	logToFilesEnabled := config.GetBool("log.useRollingLogFile")

	level, err := logging.LogLevel(config.GetString("log.level"))
	if err == nil {
		consoleBackEndLeveled := logging.AddModuleLevel(consoleBackEnd)
		consoleBackEndLeveled.SetLevel(level, "hercules")

		if logToFilesEnabled {
			normalUsageRollingLogBackEnd := logging.NewLogBackend(&lumberjack.Logger{
				Filename:   config.GetString("log.logFile"),
				MaxSize:    config.GetInt("log.maxLogFileSize"), // megabytes
				MaxBackups: config.GetInt("log.maxLogFilesToKeep"),
				Compress:   true, // disabled by default
			}, "", 0)
			normalUsageRollingLogBackEndLeveled := logging.AddModuleLevel(normalUsageRollingLogBackEnd)
			normalUsageRollingLogBackEndLeveled.SetLevel(level, "hercules")

			errorRollingLogBackEnd := logging.NewLogBackend(&lumberjack.Logger{
				Filename:   config.GetString("log.criticalErrorsLogFile"),
				MaxSize:    1, // megabytes
				MaxBackups: 1,
			}, "", 0)

			// Only critical error messages should be sent to errorRollingLogBackEndLeveled
			errorRollingLogBackEndLeveled := logging.AddModuleLevel(errorRollingLogBackEnd)
			errorRollingLogBackEndLeveled.SetLevel(logging.CRITICAL, "hercules")

			logging.SetBackend(consoleBackEndLeveled, normalUsageRollingLogBackEndLeveled, errorRollingLogBackEndLeveled)
		} else {
			logging.SetBackend(consoleBackEndLeveled)
		}

	} else {
		Log.Warningf("Could not set log level to %v: %v", config.GetString("level"), err)
		Log.Warning("Using default log level")
	}
}
