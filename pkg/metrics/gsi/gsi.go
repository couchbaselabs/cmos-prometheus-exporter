package gsi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/couchbase/tools-common/cbrest"
	"github.com/markspolakovs/yacpe/pkg/couchbase"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Metric struct {
	Name   string `json:"name"`
	Global bool   `json:"global"`
}

type MetricSet map[string]Metric

type metricInternal struct {
	gsiName string
	global  bool
	desc    *prometheus.Desc
}

type metricSetInternal map[string]*metricInternal

type Metrics struct {
	node            couchbase.NodeCommon
	msi             metricSetInternal
	mux             sync.Mutex
	logger          *zap.SugaredLogger
	FakeCollections bool
}

func (m *Metrics) Describe(descs chan<- *prometheus.Desc) {
	m.mux.Lock()
	defer m.mux.Unlock()
	for _, metric := range m.msi {
		descs <- metric.desc
	}
}

func (m *Metrics) Collect(metrics chan<- prometheus.Metric) {
	start := time.Now()
	defer func() {
		end := time.Now()
		m.logger.Debug("Completed GSI collection, took ", zap.Duration("time", end.Sub(start)))
	}()
	m.logger.Debug("Starting GSI collection")
	res, err := m.node.RestClient().Do(context.TODO(), &cbrest.Request{
		Method:             "GET",
		Endpoint:           "/api/v1/stats",
		Service:            cbrest.ServiceGSI,
		ExpectedStatusCode: http.StatusOK,
	})
	if err != nil {
		m.logger.Errorw("Failed to get GSI stats", "err", err)
		return
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		m.logger.Errorw("Failed to read GSI stats", "err", err)
		return
	}
	type gsiStatsResult map[string]map[string]interface{}
	var statsResult gsiStatsResult
	if err = json.Unmarshal(body, &statsResult); err != nil {
		m.logger.Errorw("Failed to parse GSI stats", "err", err)
		return
	}
	const statsKeyGlobal = "indexer"
	ch, err := m.getMetricsFor(statsResult[statsKeyGlobal], nil, true)
	if err != nil {
		m.logger.Errorw("Error while updating global GSI metrics", "err", err)
		return
	}
	for _, metric := range ch {
		metrics <- metric
	}
	for key, vals := range statsResult {
		if key == statsKeyGlobal {
			continue
		}
		var labels prometheus.Labels
		parts := strings.Split(key, ":")
		if len(parts) == 2 {
			labels = prometheus.Labels{
				"bucket": parts[0],
				"index":  parts[1],
			}
			if m.FakeCollections {
				labels["scope"] = "_default"
				labels["collection"] = "_default"
			}
		} else if len(parts) == 4 {
			labels = prometheus.Labels{
				"bucket":     parts[0],
				"scope":      parts[1],
				"collection": parts[2],
				"index":      parts[3],
			}
		} else {
			m.logger.Errorw("Unhandled stats name pattern", "key", key)
			return
		}
		results, err := m.getMetricsFor(vals, labels, false)
		if err != nil {
			m.logger.Errorw("While updating GSI metrics", "key", key, "err", err)
			return
		}
		for _, metric := range results {
			metrics <- metric
		}
	}
	m.logger.Debug("GSI collection done")
}

func NewMetrics(logger *zap.SugaredLogger, node couchbase.NodeCommon, ms MetricSet) (*Metrics,
	error) {
	ret := &Metrics{
		node:   node,
		msi:    make(metricSetInternal),
		logger: logger,
	}
	ret.updateMetricSet(ms)
	return ret, nil
}

func (m *Metrics) updateMetricSet(ms MetricSet) {
	m.mux.Lock()
	defer m.mux.Unlock()
	alive := make(map[string]bool)
	for key, metric := range ms {
		existing, ok := m.msi[key]
		if !ok {
			var labels []string
			if !metric.Global {
				if m.FakeCollections {
					labels = []string{"bucket", "scope", "collection", "index"}
				} else {
					labels = []string{"bucket", "index"}
				}
			}
			existing = &metricInternal{
				desc: prometheus.NewDesc(key, "", labels, nil),
			}
		}
		existing.gsiName = metric.Name
		existing.global = metric.Global
		m.msi[key] = existing
		alive[key] = true
	}
	for key := range m.msi {
		if _, ok := alive[key]; !ok {
			delete(m.msi, key)
		}
	}
}

func (m *Metrics) getMetricsFor(values map[string]interface{}, labels prometheus.Labels,
	global bool) ([]prometheus.Metric, error) {
	result := make([]prometheus.Metric, 0, len(values))
	for key, metric := range m.msi {
		if (global && !metric.global) || (!global && metric.global) {
			continue
		}
		valueTyp, ok := values[metric.gsiName]
		if !ok {
			return nil, fmt.Errorf("no GSI metric for expected %s (key %s)", metric.gsiName, key)
		}
		var value float64
		switch vt := valueTyp.(type) {
		case string:
			switch key {
			case "up":
				if vt == "Active" {
					value = 1.0
				} else {
					value = 0.0
				}
			}
		case float64:
			value = vt
		case int:
			value = float64(vt)
		default:
			return nil, fmt.Errorf("unknown type %t for value %v metric %s (%s)", valueTyp, valueTyp, metric.gsiName,
				key)
		}
		var labelValues []string
		if !metric.global {
			if m.FakeCollections {
				labelValues = []string{labels["bucket"], labels["scope"], labels["collection"], labels["index"]}
			} else {
				labelValues = []string{labels["bucket"], labels["index"]}
			}
		}
		result = append(result, prometheus.MustNewConstMetric(metric.desc, prometheus.GaugeValue, value, labelValues...))
	}
	return result, nil
}
