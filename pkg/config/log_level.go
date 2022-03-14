// Copyright 2022 Couchbase, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

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
