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

	"github.com/markspolakovs/yacpe/pkg/config"
	"github.com/markspolakovs/yacpe/pkg/couchbase"
	"github.com/markspolakovs/yacpe/pkg/metrics"
	"github.com/markspolakovs/yacpe/pkg/metrics/fts"
	"github.com/markspolakovs/yacpe/pkg/metrics/gsi"
	"github.com/markspolakovs/yacpe/pkg/metrics/memcached"
	"github.com/markspolakovs/yacpe/pkg/metrics/n1ql"
	"github.com/markspolakovs/yacpe/pkg/metrics/system"
)

var flagConfigPath = flag.String("config-file", "", "path to read config from (leave blank to use defaults)")

func main() {
	flag.Parse()

	// Set up JWW (used by Viper).
	// Sadly this won't get us nice JSON logging :(
	jww.SetStdoutThreshold(jww.LevelTrace)

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

	http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	logger.Info("HTTP server starting", zap.String("address", cfg.Bind))
	log.Fatal(http.ListenAndServe(cfg.Bind, nil))
}
