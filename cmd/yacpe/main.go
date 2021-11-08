package main

import (
	"flag"
	"fmt"
	"github.com/couchbase/tools-common/cbrest"
	"github.com/markspolakovs/yacpe/pkg/config"
	"github.com/markspolakovs/yacpe/pkg/couchbase"
	"github.com/markspolakovs/yacpe/pkg/metrics"
	gsi "github.com/markspolakovs/yacpe/pkg/metrics/gsi"
	"github.com/markspolakovs/yacpe/pkg/metrics/memcached"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"log"
	"net/http"
)

var flagConfigPath = flag.String("config-path", "./yacpe.yml", "path to read config from")

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

	node, err := couchbase.BootstrapNode(logger.Sugar(), cfg.CouchbaseHost, cfg.CouchbaseUsername, cfg.CouchbasePassword,
		cfg.CouchbaseManagementPort)
	if err != nil {
		log.Fatal(fmt.Errorf("bootstrapping failed: %w", err))
	}

	ms := metrics.LoadDefaultMetricSet()
	reg := prometheus.NewPedanticRegistry()

	hasKV, err := node.HasService(cbrest.ServiceData)
	if err != nil {
		log.Fatal(err)
	}
	if hasKV {
		mc, err := memcached.NewMemcachedMetrics(logger.Sugar().Named("memcached"), node, ms.Memcached)
		if err != nil {
			log.Fatal(err)
		}
		defer mc.Close()
		reg.MustRegister(mc)
	}

	hasGSI, err := node.HasService(cbrest.ServiceGSI)
	if err != nil {
		log.Fatal(err)
	}
	if hasGSI {
		gsiCollector, err := gsi.NewMetrics(logger.Sugar().Named("gsi"), node, cfg, ms.GSI)
		if err != nil {
			log.Fatal(err)
		}
		reg.MustRegister(gsiCollector)
	}

	http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	log.Printf("listening on %s", cfg.Bind)
	log.Fatal(http.ListenAndServe(cfg.Bind, nil))
}
