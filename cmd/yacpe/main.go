package main

import (
	"github.com/couchbase/tools-common/cbrest"
	"github.com/markspolakovs/yacpe/pkg/config"
	"github.com/markspolakovs/yacpe/pkg/couchbase"
	"github.com/markspolakovs/yacpe/pkg/metrics"
	gsi "github.com/markspolakovs/yacpe/pkg/metrics/gsi"
	"github.com/markspolakovs/yacpe/pkg/metrics/memcached"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"net/http"
	"time"
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
	collectors := make([]metrics.Collector, 0)

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
		collectors = append(collectors, mc)
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
		collectors = append(collectors, gsiCollector)
	}

	go func() {
		for range time.Tick(5 * time.Second) {
			for _, col := range collectors {
				if err := col.Collect(); err != nil {
					log.Fatal(err)
				}
			}
		}
	}()

	http.Handle("/metrics", promhttp.Handler())
	log.Printf("listening on %s", cfg.Bind)
	log.Fatal(http.ListenAndServe(cfg.Bind, nil))
}
