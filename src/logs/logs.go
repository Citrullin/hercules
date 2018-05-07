package logs

import (
	"github.com/op/go-logging"
	"os"
)

var Log = logging.MustGetLogger("example")
var log_format = "%{color}[%{level:.4s}] %{time:15:04:05.000000} %{id:06x} %{shortfile} [%{longpkg}] %{longfunc} -> %{color:reset}%{message}"

func Setup() {
	backend1 := logging.NewLogBackend(os.Stdout, "", 0)
	logging.SetFormatter(logging.MustStringFormatter(log_format))
	logging.SetBackend(backend1)
}