package gsi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/couchbase/tools-common/cbrest"
	"github.com/markspolakovs/yacpe/pkg/config"
	"github.com/markspolakovs/yacpe/pkg/couchbase"
	"github.com/prometheus/client_golang/prometheus"
	"io/ioutil"
	"strings"
	"sync"
)

type Metric struct {
	Name string `json:"name"`
	Global bool `json:"global"`
}

type MetricSet map[string]Metric

type metricInternal struct {
	gsiName string
	global bool
	gauge *prometheus.GaugeVec
}

type metricSetInternal map[string]*metricInternal

type Metrics struct {
	node *couchbase.Node
	msi metricSetInternal
	mux sync.Mutex
	cfg *config.Config
}

func NewMetrics(node *couchbase.Node, cfg *config.Config, ms MetricSet) (*Metrics, error) {
	ret := &Metrics{
		node: node,
		cfg: cfg,
		msi: make(metricSetInternal),
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
				if m.cfg.FakeCollections {
					labels = []string{"bucket", "scope", "collection", "index"}
				} else {
					labels = []string{"bucket", "index"}
				}
			}
			existing = &metricInternal{
				gauge: prometheus.NewGaugeVec(prometheus.GaugeOpts{
					Name: key,
				}, labels),
			}
			prometheus.MustRegister(existing.gauge)
		}
		existing.gsiName = metric.Name
		existing.global = metric.Global
		m.msi[key] = existing
		alive[key] = true
	}
	for key, metric := range m.msi {
		if _, ok := alive[key]; !ok {
			prometheus.Unregister(metric.gauge)
			delete(m.msi, key)
		}
	}
}

func (m *Metrics) updateMetrics(values map[string]interface{}, labels prometheus.Labels, global bool) error  {
	for key, metric := range m.msi {
		if (global && !metric.global) || (!global && metric.global) {
			continue
		}
		valueTyp, ok := values[metric.gsiName]
		if !ok {
			return fmt.Errorf("no GSI metric for expected %s (key %s)", metric.gsiName, key)
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
			return fmt.Errorf("unknown type %t for value %v metric %s (%s)", valueTyp, valueTyp, metric.gsiName, key)
		}
		metric.gauge.With(labels).Set(value)
	}
	return nil
}

func (m *Metrics) Collect() error {
	res, err := m.node.RestClient().Do(context.TODO(), &cbrest.Request{
		Method:   "GET",
		Endpoint: "/api/v1/stats",
		Service:  cbrest.ServiceGSI,
	})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}
	type metrics map[string]map[string]interface{}
	var val metrics
	if err = json.Unmarshal(body, &val); err != nil {
		return err
	}
	const statsKeyGlobal = "indexer"
	if err = m.updateMetrics(val[statsKeyGlobal], nil, true); err != nil {
		return fmt.Errorf("while updating global GSI metrics: %w", err)
	}
	for key, vals := range val {
		if key == statsKeyGlobal {
			continue
		}
		var labels prometheus.Labels
		parts := strings.Split(key, ":")
		if len(parts) == 2 {
			labels = prometheus.Labels{
				"bucket": parts[0],
				"index": parts[1],
			}
			if m.cfg.FakeCollections {
				labels["scope"] = "_default"
				labels["collection"] = "_default"
			}
		} else if len(parts) == 4 {
			labels = prometheus.Labels{
				"bucket": parts[0],
				"scope": parts[1],
				"collection": parts[2],
				"index": parts[3],
			}
		} else {
			return fmt.Errorf("unhandled stats name pattern: %v", key)
		}
		if err = m.updateMetrics(vals, labels, false); err != nil {
			return fmt.Errorf("whille updating GSI metrics for %v: %w", key, err)
		}
	}
	return nil
}
