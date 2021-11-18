package config

import (
	"fmt"
	"github.com/spf13/viper"
	"go.uber.org/zap/zapcore"
	"os"
)

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

func (c Config) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("CouchbaseHost", c.CouchbaseHost)
	enc.AddInt("CouchbaseManagementPort", c.CouchbaseManagementPort)
	enc.AddString("CouchbaseUsername", c.CouchbaseUsername)
	enc.AddString("CouchbasePassword", "[PRIVATE]")
	enc.AddBool("CouchbaseSSL", c.CouchbaseSSL)
	enc.AddString("Bind", c.Bind)
	enc.AddBool("FakeCollections", c.FakeCollections)
	enc.AddString("LogLevel", string(c.LogLevel))
	return nil
}

func Read(path string) (*Config, error) {

	viper.SetDefault("Bind", ":9091")

	viper.SetConfigName("yacpe")
	viper.SetConfigType("yaml")

	if path != "" {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("failed to open config file: %w", err)
		}
		defer file.Close()
		if err := viper.ReadConfig(file); err != nil {
			return nil, err
		}
	} else {
		viper.AddConfigPath("/etc/yacpe")
		viper.AddConfigPath("$HOME/.yacpe")
		viper.AddConfigPath(".")
		if err := viper.ReadInConfig(); err != nil {
			return nil, err
		}
	}

	var cfg Config
	err := viper.Unmarshal(&cfg)

	return &cfg, err
}
