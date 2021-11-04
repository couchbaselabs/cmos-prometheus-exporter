package main

import (
	"github.com/couchbase/tools-common/cbrest"
	"github.com/markspolakovs/yacpe/pkg/config"
	"github.com/markspolakovs/yacpe/pkg/couchbase"
	"github.com/markspolakovs/yacpe/pkg/metrics"
	gsi "github.com/markspolakovs/yacpe/pkg/metrics/gsi"
	"github.com/markspolakovs/yacpe/pkg/metrics/memcached"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"net/http"
)

func main() {
	cfg, err := config.Read()
	if err != nil {
		log.Fatal(err)
	}

	node, err := couchbase.BootstrapNode(cfg.CouchbaseHost, cfg.CouchbaseUsername, cfg.CouchbasePassword, cfg.CouchbaseManagementPort)
	if err != nil {
		log.Fatal(err)
	}

	ms := metrics.LoadDefaultMetricSet()
	reg := prometheus.NewPedanticRegistry()

	hasKV, err := node.HasService(cbrest.ServiceData)
	if err != nil {
		log.Fatal(err)
	}
	if hasKV {
		mc, err := memcached.NewMemcachedMetrics(node, ms.Memcached)
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
		gsiCollector, err := gsi.NewMetrics(node, cfg, ms.GSI)
		if err != nil {
			log.Fatal(err)
		}
		reg.MustRegister(gsiCollector)
	}

	http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	log.Printf("listening on %s", cfg.Bind)
	log.Fatal(http.ListenAndServe(cfg.Bind, nil))
}
