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
	"flag"
	"log"
	"net/http"

	goutilslog "github.com/couchbase/goutils/logging"
	"github.com/couchbase/tools-common/cbrest"
	toolscommonlog "github.com/couchbase/tools-common/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	jww "github.com/spf13/jwalterweatherman"
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
)

var flagConfigPath = flag.String("config-file", "", "path to read config from (leave blank to use defaults)")

func main() {
	flag.Parse()

	// Set up JWW (used by Viper).
	// Sadly this won't get us nice JSON logging :(
	jww.SetStdoutThreshold(jww.LevelDebug)

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

	logger.Debug("Loaded config", zap.Object("cfg", cfg))

	toolscommonlog.SetLogger(&config.CBLogZapLogger{Logger: logger.WithOptions(zap.AddCallerSkip(3)).Named("cb").Sugar()})
	goutilslog.SetLogger(&config.GoUtilsZapLogger{Logger: logger.WithOptions(zap.AddCallerSkip(2)).Named(
		"memcached").Sugar()})

	node, err := couchbase.BootstrapNode(logger.Sugar(), cfg.CouchbaseHost, cfg.CouchbaseUsername, cfg.CouchbasePassword,
		cfg.CouchbaseManagementPort)
	if err != nil {
		logger.Sugar().Fatalw("Failed to bootstrap cluster", "err", err)
	}

	ms := metrics.LoadDefaultMetricSet()
	reg := prometheus.NewPedanticRegistry()

	sys := system.NewSystemMetrics(logger.Named("system").Sugar(), ms.System)
	reg.MustRegister(sys)

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
	}

	hasFTS, err := node.HasService(cbrest.ServiceSearch)
	if err != nil {
		logger.Sugar().Fatalw("Failed to check FTS", "err", err)
	}
	if hasFTS {
		ftsCollector := fts.NewCollector(logger.Sugar().Named("fts"), node, ms.FTS, cfg.FakeCollections)
		reg.MustRegister(ftsCollector)
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
	}

	http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	logger.Info("HTTP server starting", zap.String("address", cfg.Bind))
	log.Fatal(http.ListenAndServe(cfg.Bind, nil))
}
