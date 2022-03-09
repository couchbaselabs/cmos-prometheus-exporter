package fts

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/couchbase/tools-common/cbrest"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"

	"github.com/couchbaselabs/cmos-prometheus-exporter/pkg/couchbase"
)

type Metric struct {
	Name   string `json:"name"`
	Global bool   `json:"global"`
}

type MetricSet map[string]Metric

type metricInternal struct {
	Metric
	desc    *prometheus.Desc
	ftsName string
}

// NOTE: metricSetInternal is keyed by FTS name, *not* Prometheus name.
type metricSetInternal map[string]*metricInternal

type Collector struct {
	logger          *zap.SugaredLogger
	node            couchbase.NodeCommon
	msi             metricSetInternal
	msiMux          sync.RWMutex
	fakeCollections bool
}

func NewCollector(logger *zap.SugaredLogger, node couchbase.NodeCommon, metrics MetricSet, fakeCollections bool) *Collector {
	c := &Collector{
		logger:          logger,
		node:            node,
		fakeCollections: fakeCollections,
		msi:             make(metricSetInternal),
	}
	c.updateMSI(metrics)
	return c
}

func (c *Collector) Describe(descs chan<- *prometheus.Desc) {
	c.msiMux.RLock()
	defer c.msiMux.RUnlock()
	for _, metric := range c.msi {
		descs <- metric.desc
	}
}

var singleIndexStatRe = regexp.MustCompile(`^(?P<bucket>.+?):(?P<index>.+?):(?P<stat>.+)$`)

func (c *Collector) Collect(metrics chan<- prometheus.Metric) {
	start := time.Now()
	c.logger.Info("Starting FTS collection")
	defer func() {
		c.logger.Infow("Completed FTS collection", "elapsed", time.Since(start))
	}()
	c.msiMux.RLock()
	defer c.msiMux.RUnlock()

	response, err := c.node.RestClient().Execute(&cbrest.Request{
		Method:             http.MethodGet,
		Endpoint:           "/api/nsstats",
		Service:            cbrest.ServiceSearch,
		ExpectedStatusCode: http.StatusOK,
		Idempotent:         true,
	})
	if err != nil {
		c.logger.Errorw("Failed to get FTS stats", "error", err)
		return
	}

	// most stats are float64s, but some are strings
	var stats map[string]interface{}
	if err := json.Unmarshal(response.Body, &stats); err != nil {
		c.logger.Errorw("Failed to unmarshal FTS stats", "error", err)
		return
	}

	for key, rawValue := range stats {
		var (
			metric *metricInternal
			ok     bool
		)
		statMatch := singleIndexStatRe.FindStringSubmatch(key)
		labels := make(prometheus.Labels)
		if statMatch != nil {
			metric, ok = c.msi[statMatch[singleIndexStatRe.SubexpIndex("stat")]]
			labels["bucket"] = statMatch[singleIndexStatRe.SubexpIndex("bucket")]
			labels["index"] = statMatch[singleIndexStatRe.SubexpIndex("index")]
			if c.fakeCollections {
				labels["scope"] = "_default"
				labels["collection"] = "_default"
			}
		} else {
			metric, ok = c.msi[key]
		}
		if !ok {
			continue
		}
		var value float64
		switch valueTyp := rawValue.(type) {
		case float64:
			value = valueTyp
		case int64:
			value = float64(valueTyp)
		default:
			c.logger.Errorw("Got unexpected value type", "key", key, "value", valueTyp, "valueType", fmt.Sprintf("%T", valueTyp))
		}
		var labelValues []string
		if !metric.Global {
			if c.fakeCollections {
				labelValues = []string{labels["bucket"], labels["scope"], labels["collection"], labels["index"]}
			} else {
				labelValues = []string{labels["bucket"], labels["index"]}
			}
		}
		metrics <- prometheus.MustNewConstMetric(metric.desc, prometheus.UntypedValue, value, labelValues...)
	}
}

func (c *Collector) updateMSI(ms MetricSet) {
	c.msiMux.Lock()
	defer c.msiMux.Unlock()
	alive := make(map[string]bool)
	for key, metric := range ms {
		existing, ok := c.msi[metric.Name]
		if !ok {
			var labels []string
			if !metric.Global {
				if c.fakeCollections {
					labels = []string{"bucket", "scope", "collection", "index"}
				} else {
					labels = []string{"bucket", "index"}
				}
			}
			existing = &metricInternal{
				desc:   prometheus.NewDesc(key, "", labels, nil),
				Metric: metric,
			}
		}
		existing.ftsName = metric.Name
		c.msi[metric.Name] = existing
		alive[metric.Name] = true
	}
	for key := range c.msi {
		if _, ok := alive[key]; !ok {
			delete(c.msi, key)
		}
	}
}
