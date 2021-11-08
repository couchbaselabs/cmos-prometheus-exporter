package main

import (
	"flag"
	goutilslog "github.com/couchbase/goutils/logging"
	"github.com/couchbase/tools-common/cbrest"
	toolscommonlog "github.com/couchbase/tools-common/log"
	"github.com/markspolakovs/yacpe/pkg/config"
	"github.com/markspolakovs/yacpe/pkg/couchbase"
	"github.com/markspolakovs/yacpe/pkg/metrics"
	gsi "github.com/markspolakovs/yacpe/pkg/metrics/gsi"
	"github.com/markspolakovs/yacpe/pkg/metrics/memcached"
	"github.com/markspolakovs/yacpe/pkg/metrics/n1ql"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"log"
	"net/http"
)

var flagConfigPath = flag.String("config-file", "", "path to read config from (leave blank to use defaults)")

func main() {
	flag.Parse()

	cfg, err := config.Read(*flagConfigPath)
	if err != nil {
		log.Fatal(err)
	}

	// From this point on, we should switch to using Zap for logging.
	logCfg := zap.NewProductionConfig()
	logCfg.Level = zap.NewAtomicLevelAt(cfg.LogLevel.ToZap())
	logger, _ := logCfg.Build()
	defer logger.Sync()

	logger.Sugar().Debugw("Loaded config", "cfg", cfg)

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
		reg.MustRegister(mc)
	}

	hasGSI, err := node.HasService(cbrest.ServiceGSI)
	if err != nil {
		logger.Sugar().Fatalw("Failed to check GSI", "err", err)
	}
	if hasGSI {
		gsiCollector, err := gsi.NewMetrics(logger.Sugar().Named("gsi"), node, cfg, ms.GSI)
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
		n1qlCollector, err := n1ql.NewMetrics(logger.Sugar().Named("n1ql"), node, cfg, ms.N1QL)
		if err != nil {
			logger.Sugar().Fatalw("Failed to create N1QL collector", "err", err)
		}
		reg.MustRegister(n1qlCollector)
	}

	http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	logger.Info("HTTP server starting", zap.String("address", cfg.Bind))
	log.Fatal(http.ListenAndServe(cfg.Bind, nil))
}
