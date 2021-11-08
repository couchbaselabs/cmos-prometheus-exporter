package couchbase

import (
	"github.com/couchbase/tools-common/log"
	"go.uber.org/zap"
)

type CBLogZapLogger struct {
	logger *zap.SugaredLogger
}

func (c *CBLogZapLogger) Log(level log.Level, format string, args ...interface{}) {
	switch level {
	case log.LevelTrace:
		fallthrough
	case log.LevelDebug:
		c.logger.Debugf(format, args...)
	case log.LevelInfo:
		c.logger.Infof(format, args...)
	case log.LevelWarning:
		c.logger.Warnf(format, args...)
	case log.LevelError:
		c.logger.Errorf(format, args...)
	case log.LevelPanic:
		c.logger.Panicf(format, args...)
	}
}
