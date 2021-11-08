package config

import (
	"fmt"
	"github.com/couchbase/tools-common/log"
	"github.com/spf13/viper"
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

type Config struct {
	CouchbaseHost           string   `mapstructure:"couchbase_host"`
	CouchbaseManagementPort int      `mapstructure:"couchbase_management_port"`
	CouchbaseUsername       string   `mapstructure:"couchbase_username"`
	CouchbasePassword       string   `mapstructure:"couchbase_password"`
	CouchbaseSSL            bool     `mapstructure:"couchbase_ssl"`
	Bind                    string   `mapstructure:"bind"`
	FakeCollections         bool     `mapstructure:"fake_collections"`
	LogLevel                LogLevel `mapstructure:"log_level"`
}

func Read(path string) (*Config, error) {

	viper.SetDefault("Bind", ":9091")

	viper.SetConfigName("yacpe")
	viper.AddConfigPath(".")
	viper.SetConfigType("yaml")

	if path != "" {
		viper.AddConfigPath(path)
	}

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read in config: %w", err)
		}
	}

	var cfg Config
	err := viper.Unmarshal(&cfg)

	return &cfg, err
}
