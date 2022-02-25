package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
	"go.uber.org/zap/zapcore"
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
	viper.SetDefault("couchbase_host", "localhost")
	viper.SetDefault("couchbase_management_port", 8091)
	viper.SetDefault("bind", ":9091")
	viper.SetDefault("fake_collections", true)
	viper.SetDefault("log_level", "info")

	viper.SetConfigName("cmos-exporter")
	viper.SetConfigType("yaml")

	viper.SetEnvPrefix("CMOS_EXPORTER")
	viper.AutomaticEnv()

	_ = viper.BindEnv("couchbase_username", "COUCHBASE_OPERATOR_USER", "CMOS_EXPORTER_COUCHBASE_USERNAME")
	_ = viper.BindEnv("couchbase_password", "COUCHBASE_OPERATOR_PASS", "CMOS_EXPORTER_COUCHBASE_PASSWORD")

	//nolint:nestif
	if path != "" {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("failed to open config file: %w", err)
		}
		defer file.Close()
		if err := viper.ReadConfig(file); err != nil {
			return nil, fmt.Errorf("failed to read non-default config: %w", err)
		}
	} else {
		viper.AddConfigPath("/etc/cmos-exporter")
		viper.AddConfigPath(".")
		if err := viper.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				return nil, fmt.Errorf("failed to read default config paths: %w", err)
			}
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}
