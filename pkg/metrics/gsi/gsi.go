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

package gsi

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
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
	fakeCollections bool
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
		m.logger.Infow("Completed GSI collection", zap.Duration("elapsed", end.Sub(start)))
	}()
	m.logger.Info("Starting GSI collection")
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
			if m.fakeCollections {
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

func NewMetrics(logger *zap.SugaredLogger, node couchbase.NodeCommon, ms MetricSet, fakeCollections bool) (*Metrics,
	error,
) {
	ret := &Metrics{
		node:            node,
		msi:             make(metricSetInternal),
		logger:          logger,
		fakeCollections: fakeCollections,
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
				if m.fakeCollections {
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
	global bool,
) ([]prometheus.Metric, error) {
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
			if m.fakeCollections {
				labelValues = []string{labels["bucket"], labels["scope"], labels["collection"], labels["index"]}
			} else {
				labelValues = []string{labels["bucket"], labels["index"]}
			}
		}
		m.logger.Desugar().Debug("Mapped metric", zap.String("gsiName", key), zap.String("desc", metric.desc.String()),
			zap.Strings("labels", labelValues))
		result = append(result, prometheus.MustNewConstMetric(metric.desc, prometheus.GaugeValue, value, labelValues...))
	}
	return result, nil
}
