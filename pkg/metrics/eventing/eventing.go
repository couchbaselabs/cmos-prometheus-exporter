package eventing

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/couchbase/tools-common/cbrest"
	"github.com/itchyny/gojq"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"

	"github.com/couchbaselabs/cmos-prometheus-exporter/pkg/couchbase"
)

type Metric struct {
	// Eventing metrics function a little differently to others, since they're an object, rather than just a flat list of
	// stats. The expression is a JQ-like (https://github.com/itchyny/gojq) expression that must evaluate to an array,
	// where the first element is a number (the stat value) and all others are strings, which will become labels (in the
	// same order as the labels array).
	Expression  string            `json:"expression"`
	Help        string            `json:"help"`
	Labels      []string          `json:"labels"`
	ConstLabels prometheus.Labels `json:"constLabels"`
}

type MetricSet map[string]Metric

type metricInternal struct {
	Metric
	desc *prometheus.Desc
	expr *gojq.Code
}

type metricSetInternal map[string]metricInternal

type Metrics struct {
	logger *zap.SugaredLogger
	node   couchbase.NodeCommon
	msi    metricSetInternal
	msiMux sync.RWMutex
}

func NewCollector(logger *zap.SugaredLogger, node couchbase.NodeCommon, metrics MetricSet) (*Metrics, error) {
	collector := &Metrics{
		logger: logger,
		node:   node,
		msi:    make(metricSetInternal),
	}
	return collector, collector.updateMSI(metrics)
}

func (m *Metrics) Describe(descs chan<- *prometheus.Desc) {
	m.msiMux.RLock()
	defer m.msiMux.RUnlock()
	for _, metric := range m.msi {
		descs <- metric.desc
	}
}

func (m *Metrics) Collect(metrics chan<- prometheus.Metric) {
	start := time.Now()
	m.logger.Info("Starting Eventing collection")
	defer func() {
		m.logger.Infow("Completed Eventing collection", "elapsed", time.Since(start))
	}()
	m.msiMux.RLock()
	defer m.msiMux.RUnlock()
	res, err := m.node.RestClient().Execute(&cbrest.Request{
		Method:             http.MethodGet,
		Service:            cbrest.ServiceEventing,
		Endpoint:           "/api/v1/stats",
		ExpectedStatusCode: http.StatusOK,
		Idempotent:         true,
	})
	if err != nil {
		m.logger.Errorw("Failed to collect metrics", "error", err)
		return
	}

	// Metrics are an array of stats for each function
	var metricValues []interface{}
	if err := json.Unmarshal(res.Body, &metricValues); err != nil {
		m.logger.Errorw("Failed to unmarshal metrics", "error", err)
		return
	}

	for key, metric := range m.msi {
		results := metric.expr.Run(metricValues)
		for {
			row, ok := results.Next()
			if !ok {
				break
			}
			if err, ok := row.(error); ok {
				m.logger.Warnw("Error when evaluating expression", "metric", key, "error", err)
				continue
			}
			m.logger.Debugw("Expression result", "metric", key, "value", row)
			result, ok := row.([]interface{})
			if !ok {
				m.logger.Warnw("Expression did not evaluate to an array", "metric", key, "value", fmt.Sprintf("%#v", result))
				continue
			}
			value, ok := result[0].(float64)
			if !ok {
				m.logger.Warnw("Expression's first result was not a float64", "metric", key, "value", fmt.Sprintf("%#v", result[0]))
				continue
			}
			labels := make([]string, 0, len(result)-1)
			for i, label := range result[1:] {
				labelValue, ok := label.(string)
				if !ok {
					m.logger.Warnw("Expression's label result was not a string", "metric", key, "i", i, "value", fmt.Sprintf("%#v", label))
					continue
				}
				labels = append(labels, labelValue)
			}

			metrics <- prometheus.MustNewConstMetric(metric.desc, prometheus.UntypedValue, value, labels...)
		}
	}
}

func (m *Metrics) updateMSI(metrics MetricSet) error {
	m.msiMux.Lock()
	defer m.msiMux.Unlock()
	alive := make(map[string]bool)
	for key, metric := range metrics {
		existing, ok := m.msi[key]
		if !ok {
			existing = metricInternal{
				Metric: metric,
				desc:   prometheus.NewDesc(key, metric.Help, metric.Labels, metric.ConstLabels),
			}
		}
		query, err := gojq.Parse(metric.Expression)
		if err != nil {
			return fmt.Errorf("invalid expression for metric %s: %w", key, err)
		}
		code, err := gojq.Compile(query)
		if err != nil {
			return fmt.Errorf("failed to compile query for metric %s: %w", key, err)
		}
		existing.expr = code
		m.msi[key] = existing
		alive[key] = true
	}
	for key := range m.msi {
		if _, ok := alive[key]; !ok {
			delete(m.msi, key)
		}
	}
	return nil
}
