package config

import (
	"fmt"
	"github.com/spf13/viper"
)

type Config struct {
	CouchbaseHost string `mapstructure:"couchbase_host"`
	CouchbaseManagementPort int `mapstructure:"couchbase_management_port"`
	CouchbaseUsername string `mapstructure:"couchbase_username"`
	CouchbasePassword string `mapstructure:"couchbase_password"`
	CouchbaseSSL bool `mapstructure:"couchbase_ssl"`
	Bind string `mapstructure:"bind"`
	FakeCollections bool `mapstructure:"fake_collections"`
}

func Read() (*Config, error) {
	viper.SetDefault("Bind", ":9091")

	viper.SetConfigName("yacpe")
	viper.AddConfigPath(".")
	viper.SetConfigType("yaml")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read in config: %w", err)
		}
	}

	viper.SetEnvPrefix("YACPE_")
	viper.AutomaticEnv()

	var cfg Config
	err := viper.Unmarshal(&cfg)

	return &cfg, err
}
