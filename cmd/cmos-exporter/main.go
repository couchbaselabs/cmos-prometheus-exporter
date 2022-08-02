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

package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"runtime/debug"

	goutilslog "github.com/couchbase/goutils/logging"
	"github.com/couchbase/tools-common/cbrest"
	toolscommonlog "github.com/couchbase/tools-common/log"
	"github.com/couchbaselabs/cmos-prometheus-exporter/pkg/meta"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	jww "github.com/spf13/jwalterweatherman"
	"github.com/spf13/pflag"
	"go.uber.org/zap"

	"github.com/couchbaselabs/cmos-prometheus-exporter/pkg/config"
	"github.com/couchbaselabs/cmos-prometheus-exporter/pkg/couchbase"
	"github.com/couchbaselabs/cmos-prometheus-exporter/pkg/metrics"
	"github.com/couchbaselabs/cmos-prometheus-exporter/pkg/metrics/eventing"
	"github.com/couchbaselabs/cmos-prometheus-exporter/pkg/metrics/fts"
	"github.com/couchbaselabs/cmos-prometheus-exporter/pkg/metrics/gsi"
	"github.com/couchbaselabs/cmos-prometheus-exporter/pkg/metrics/memcached"
	"github.com/couchbaselabs/cmos-prometheus-exporter/pkg/metrics/n1ql"
	"github.com/couchbaselabs/cmos-prometheus-exporter/pkg/metrics/system"
	"github.com/couchbaselabs/cmos-prometheus-exporter/pkg/metrics/xdcr"
)

var (
	flagConfigPath = pflag.StringP("config_file", "c", "", "path to read config from (leave blank to use defaults)")
	flagVersion    = pflag.BoolP("version", "v", false, "print the version, then exit")
)

func buildSettingsToMap(bs []debug.BuildSetting) map[string]string {
	result := make(map[string]string, len(bs))
	for _, val := range bs {
		result[val.Key] = val.Value
	}
	return result
}

func processBuildInfo() map[string]string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return nil
	}
	settings := buildSettingsToMap(info.Settings)
	result := make(map[string]string)
	result["go"] = info.GoVersion
	result["os"] = settings["GOOS"]
	result["arch"] = settings["GOARCH"]
	result["compiler"] = settings["-compiler"]
	result["rev"] = settings["vcs.revision"]
	return result
}

func main() {
	buildInfo := processBuildInfo()
	pflag.Parse()

	if *flagVersion {
		fmt.Printf("cmos-prometheus-exporter version %s (revision %s)\n", meta.Version, buildInfo["rev"])
		os.Exit(0)
	}

	// Set up JWW (used by Viper).
	// Sadly this won't get us nice JSON logging :(
	jww.SetStdoutThreshold(jww.LevelInfo)

	cfg, err := config.Read(*flagConfigPath)
	if err != nil {
		log.Fatal(err)
	}

	// From this point on, we should switch to using Zap for logging.
	logCfg := zap.NewProductionConfig()
	logCfg.Level = zap.NewAtomicLevelAt(cfg.LogLevel.ToZap())
	logCfg.Encoding = "console"
	logger, _ := logCfg.Build()
	defer logger.Sync()

	logger.Info("Started & configured logging", zap.String("version", meta.Version), zap.Any("buildInfo", buildInfo))
	// using zap.Object ensures we don't leak any sensitive fields, as the ObjectMarshaller will redact them
	logger.Debug("Loaded configuration", zap.Object("cfg", cfg))

	toolscommonlog.SetLogger(&config.CBLogZapLogger{Logger: logger.WithOptions(zap.AddCallerSkip(3)).Named("cb").Sugar()})
	goutilslog.SetLogger(&config.GoUtilsZapLogger{Logger: logger.WithOptions(zap.AddCallerSkip(2)).Named(
		"memcached").Sugar()})

	node, err := couchbase.BootstrapNode(logger.Sugar(), couchbase.BootstrapNodeOptions{
		ConnectionString:   cfg.CouchbaseConnectionString,
		Username:           cfg.CouchbaseUsername,
		Password:           cfg.CouchbasePassword,
		CACertFile:         cfg.CouchbaseCACertFile,
		ClientCertFile:     cfg.CouchbaseClientCertFile,
		KeyFile:            cfg.CouchbaseClientKeyFile,
		InsecureSkipVerify: cfg.InsecureCouchbaseSkipTLSVerify,
	})
	if err != nil {
		logger.Sugar().Fatalw("Failed to bootstrap cluster", "err", err)
	}

	ms := metrics.LoadDefaultMetricSet()
	reg := prometheus.NewPedanticRegistry()

	sys := system.NewSystemMetrics(logger.Named("system").Sugar(), ms.System)
	reg.MustRegister(sys)
	logger.Info("Registered system collector")

	nodeIP := net.ParseIP(node.Hostname())
	if nodeIP == nil {
		ips, err := net.LookupIP(node.Hostname())
		if err != nil {
			logger.Fatal("Failed to look up the host IP", zap.String("host", node.Hostname()), zap.Error(err))
		}
		if len(ips) == 0 {
			logger.Fatal("Found no IPs for host", zap.String("host", node.Hostname()))
		}
		nodeIP = ips[0]
	}
	if nodeIP != nil && nodeIP.IsLoopback() {
		xdcrColl, err := xdcr.NewXDCRMetrics(logger.Named("xdcr").Sugar(), node, ms.XDCR)
		if err != nil {
			logger.Sugar().Fatalw("Failed to create XDCR collector", "err", err)
		}
		reg.MustRegister(xdcrColl)
		logger.Info("Registered XDCR collector")
	} else {
		logger.Warn("Node hostname is not loopback - XDCR metrics are only available when running on localhost")
	}

	hasKV, err := node.HasService(cbrest.ServiceData)
	if err != nil {
		logger.Sugar().Fatalw("Failed to check KV", "err", err)
	}
	if hasKV {
		mc, err := memcached.NewMemcachedMetrics(logger.Named("memcached"), node, ms.Memcached)
		if err != nil {
			logger.Sugar().Fatalw("Failed to create memcached collector", "err", err)
		}
		defer mc.Close()
		// TODO: we need to add scope/collection labels to the various metrics
		// mc.FakeCollections = cfg.FakeCollections
		reg.MustRegister(mc)
		logger.Info("Registered memcached collector")
	}

	hasGSI, err := node.HasService(cbrest.ServiceGSI)
	if err != nil {
		logger.Sugar().Fatalw("Failed to check GSI", "err", err)
	}
	if hasGSI {
		gsiCollector, err := gsi.NewMetrics(logger.Sugar().Named("gsi"), node, ms.GSI, cfg.FakeCollections)
		if err != nil {
			logger.Sugar().Fatalw("Failed to create GSI collector", "err", err)
		}
		reg.MustRegister(gsiCollector)
		logger.Info("Registered GSI collector")
	}

	hasN1QL, err := node.HasService(cbrest.ServiceQuery)
	if err != nil {
		logger.Sugar().Fatalw("Failed to check N1QL", "err", err)
	}
	if hasN1QL {
		n1qlCollector, err := n1ql.NewMetrics(logger.Sugar().Named("n1ql"), node, ms.N1QL)
		if err != nil {
			logger.Sugar().Fatalw("Failed to create N1QL collector", "err", err)
		}
		reg.MustRegister(n1qlCollector)
		logger.Info("Registered N1QL collector")
	}

	hasFTS, err := node.HasService(cbrest.ServiceSearch)
	if err != nil {
		logger.Sugar().Fatalw("Failed to check FTS", "err", err)
	}
	if hasFTS {
		ftsCollector := fts.NewCollector(logger.Sugar().Named("fts"), node, ms.FTS, cfg.FakeCollections)
		reg.MustRegister(ftsCollector)
		logger.Info("Registered FTS collector")
	}

	hasEventing, err := node.HasService(cbrest.ServiceEventing)
	if err != nil {
		logger.Sugar().Fatalw("Failed to check Eventing", "err", err)
	}
	if hasEventing {
		eventingCollector, err := eventing.NewCollector(logger.Sugar().Named("eventing"), node, ms.Eventing)
		if err != nil {
			logger.Sugar().Fatalw("Failed to create Eventing collector", "err", err)
		}
		reg.MustRegister(eventingCollector)
		logger.Info("Registered Eventing collector")
	}

	http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	logger.Info("HTTP server starting", zap.String("address", cfg.Bind))
	log.Fatal(http.ListenAndServe(cfg.Bind, nil))
}
