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
