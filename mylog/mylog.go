package mylog

import (
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/glog"
)

var logger *glog.Logger

var DevConfig = g.Map{
	"path":       "./log",
	"level":      "DEBUG",
	"stdout":     true,
	"stStatus":   1,
	"timeFormat": time.DateTime,
}

var ProdConfig = g.Map{
	"path":       "./log",
	"level":      "ERROR",
	"stdout":     false,
	"stStatus":   1,
	"timeFormat": time.DateTime,
}

func Init(conf ...g.Map) error {
	logger = glog.New()
	var c g.Map
	if len(conf) == 0 {
		c = ProdConfig
	} else {
		c = conf[0]
	}
	return logger.SetConfigWithMap(c)
}

func Logger() *glog.Logger {
	return logger
}
