package n1ql

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"github.com/couchbase/tools-common/cbrest"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"

	"github.com/couchbaselabs/cmos-prometheus-exporter/pkg/couchbase"
	"github.com/couchbaselabs/cmos-prometheus-exporter/pkg/metrics/common"
)

type Metric struct {
	Name string            `json:"name"`
	Help string            `json:"help"`
	Type common.MetricType `json:"type"`
}

type MetricSet map[string]Metric

type metricInternal struct {
	n1qlName   string
	desc       *prometheus.Desc
	metricType common.MetricType
}

// Note: this is keyed by n1ql name, NOT prometheus name
type metricSetInternal map[string]*metricInternal

type Metrics struct {
	ms MetricSet
	// Note: this is keyed by n1ql name, NOT prometheus name
	msi    metricSetInternal
	node   couchbase.NodeCommon
	mux    sync.Mutex
	logger *zap.Logger
}

func NewMetrics(logger *zap.SugaredLogger, node couchbase.NodeCommon, ms MetricSet) (*Metrics, error) {
	ret := &Metrics{
		node:   node,
		msi:    make(metricSetInternal),
		logger: logger.Desugar(),
	}
	ret.updateMetricSet(ms)
	return ret, nil
}

func (m *Metrics) Describe(descs chan<- *prometheus.Desc) {
	for _, metric := range m.msi {
		descs <- metric.desc
	}
}

func (m *Metrics) Collect(metrics chan<- prometheus.Metric) {
	start := time.Now()
	defer func() {
		end := time.Now()
		m.logger.Debug("Completed N1QL collection, took ", zap.Duration("time", end.Sub(start)))
	}()
	m.logger.Debug("Starting N1QL collection")
	m.mux.Lock()
	defer m.mux.Unlock()
	res, err := m.node.RestClient().Do(context.TODO(), &cbrest.Request{
		Method:             "GET",
		Endpoint:           "/admin/stats",
		Service:            cbrest.ServiceQuery,
		ExpectedStatusCode: http.StatusOK,
	})
	if err != nil {
		m.logger.Sugar().Errorw("Failed to get N1QL stats", "err", err)
		return
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		m.logger.Sugar().Errorw("Failed to read N1QL stats", "err", err)
		return
	}
	type n1qlResult map[string]float64
	var result n1qlResult
	if err := json.Unmarshal(body, &result); err != nil {
		m.logger.Sugar().Errorw("Failed to parse N1QL stats", "err", err)
		return
	}

	for stat, value := range result {
		metric, ok := m.msi[stat]
		if ok {
			metrics <- prometheus.MustNewConstMetric(
				metric.desc,
				metric.metricType.ToPrometheus(),
				value,
			)
		}
	}
	m.logger.Debug("N1QL collection complete")
}

func (m *Metrics) updateMetricSet(ms MetricSet) {
	m.mux.Lock()
	defer m.mux.Unlock()

	msi := make(metricSetInternal)
	for promName, metric := range ms {
		msi[metric.Name] = &metricInternal{
			n1qlName:   metric.Name,
			desc:       prometheus.NewDesc(promName, metric.Help, nil, nil),
			metricType: metric.Type,
		}
	}

	m.ms = ms
	m.msi = msi
}
