package config

import (
	"github.com/couchbase/goutils/logging"
	"github.com/couchbase/tools-common/log"
	"go.uber.org/zap/zapcore"
)

type LogLevel string

const (
	Trace   LogLevel = "trace"
	Debug   LogLevel = "debug"
	Info    LogLevel = "info"
	Warning LogLevel = "warning"
	Error   LogLevel = "error"
	Panic   LogLevel = "panic"
)

func (l LogLevel) ToCBLog() log.Level {
	switch l {
	case Trace:
		return log.LevelTrace
	case Debug:
		return log.LevelDebug
	case Info:
		return log.LevelInfo
	case Warning:
		return log.LevelWarning
	case Error:
		return log.LevelError
	case Panic:
		return log.LevelPanic
	default:
		return log.LevelInfo
	}
}

func (l LogLevel) ToZap() zapcore.Level {
	switch l {
	case Trace:
		return zapcore.DebugLevel
	case Debug:
		return zapcore.DebugLevel
	case Info:
		return zapcore.InfoLevel
	case Warning:
		return zapcore.WarnLevel
	case Error:
		return zapcore.ErrorLevel
	case Panic:
		return zapcore.PanicLevel
	default:
		return zapcore.InfoLevel
	}
}

func (l LogLevel) ToGoUtils() logging.Level {
	switch l {
	case Trace:
		return logging.TRACE
	case Debug:
		return logging.DEBUG
	case Info:
		return logging.INFO
	case Warning:
		return logging.WARN
	case Error:
		return logging.ERROR
	case Panic:
		return logging.FATAL
	default:
		return logging.INFO
	}
}
