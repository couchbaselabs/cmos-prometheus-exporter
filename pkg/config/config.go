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
	"fmt"
	"os"

	"github.com/spf13/pflag"
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

func init() {
	pflag.StringP("couchbase_host", "h", "localhost", "hostname of Couchbase Server")
	pflag.IntP("couchbase_management_port", "p", 8091, "management port of Couchbase Server")
	pflag.StringP("couchbase_user", "u", "Administrator", "username to connect to - must be Read-Only Admin")
	pflag.StringP("couchbase_password", "P", "", "password to use")
	pflag.BoolP("couchbase_ssl", "s", false, "whether to require TLS")
	pflag.StringP("bind", "b", ":9091", "host:port to serve on")
	pflag.Bool("fake_collections", false, "whether to add scope/collection labels to metrics that use them")
	pflag.StringP("log_level", "l", "info", "level to log at")
}

func (c Config) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("CouchbaseHost", c.CouchbaseHost)
	enc.AddInt("CouchbaseManagementPort", c.CouchbaseManagementPort)
	enc.AddString("CouchbaseUsername", "<ud>"+c.CouchbaseUsername+"</ud>")
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

	_ = viper.BindPFlags(pflag.CommandLine)

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
