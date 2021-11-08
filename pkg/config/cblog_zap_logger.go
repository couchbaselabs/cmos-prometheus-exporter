package config

import (
	"github.com/couchbase/tools-common/log"
	"go.uber.org/zap"
)

type CBLogZapLogger struct {
	Logger *zap.SugaredLogger
}

func (c *CBLogZapLogger) Log(level log.Level, format string, args ...interface{}) {
	switch level {
	case log.LevelTrace:
		fallthrough
	case log.LevelDebug:
		c.Logger.Debugf(format, args...)
	case log.LevelInfo:
		c.Logger.Infof(format, args...)
	case log.LevelWarning:
		c.Logger.Warnf(format, args...)
	case log.LevelError:
		c.Logger.Errorf(format, args...)
	case log.LevelPanic:
		c.Logger.Panicf(format, args...)
	}
}
