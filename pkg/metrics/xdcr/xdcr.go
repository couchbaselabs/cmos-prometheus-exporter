package xdcr

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
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
	Metric
	desc *prometheus.Desc
}

type metricSetInternal map[string]*metricInternal

// TODO: don't hard-code this
const xdcrRestPort = 9998

type Metrics struct {
	logger *zap.SugaredLogger
	node   couchbase.NodeCommon
	msi    metricSetInternal
	mux    sync.RWMutex
}

var labelNames = []string{"targetClusterUUID", "sourceBucketName", "targetBucketName", "pipelineType"}

func NewXDCRMetrics(logger *zap.SugaredLogger, node couchbase.NodeCommon, metricSet MetricSet) (*Metrics, error) {
	coll := &Metrics{
		logger: logger,
		node:   node,
		msi:    make(metricSetInternal),
	}
	coll.updateMSI(metricSet)
	return coll, nil
}

func (m *Metrics) Describe(descs chan<- *prometheus.Desc) {
	m.mux.RLock()
	defer m.mux.RUnlock()
	for _, metric := range m.msi {
		descs <- metric.desc
	}
}

func (m *Metrics) Collect(metrics chan<- prometheus.Metric) {
	start := time.Now()
	defer func() {
		end := time.Now()
		m.logger.Debug("Completed XDCR collection, took ", zap.Duration("time", end.Sub(start)))
	}()
	m.logger.Debug("Starting XDCR collection")
	m.mux.RLock()
	defer m.mux.RUnlock()
	// Get the list of all configured replications, then get the stats for each source bucket
	repsRes, err := m.node.RestClient().Execute(&cbrest.Request{
		Method:             http.MethodGet,
		Service:            cbrest.ServiceManagement,
		Endpoint:           "/pools/default/replications",
		ExpectedStatusCode: http.StatusOK,
		Idempotent:         true,
	})
	if err != nil {
		m.logger.Errorw("Failed to get configured replications", "error", err)
		return
	}

	var replicationsData []struct {
		SourceBucket string `json:"source"`
	}
	if err := json.Unmarshal(repsRes.Body, &replicationsData); err != nil {
		m.logger.Errorw("Failed to parse configured replications", "error", err)
		return
	}

	for _, rep := range replicationsData {
		m.processStatsForReplication(rep.SourceBucket, metrics)
	}
}

func (m *Metrics) updateMSI(ms MetricSet) {
	m.mux.Lock()
	defer m.mux.Unlock()
	alive := make(map[string]bool)
	for key, metric := range ms {
		existing, ok := m.msi[key]
		if !ok {
			existing = &metricInternal{
				Metric: metric,
				desc:   prometheus.NewDesc(key, metric.Help, labelNames, nil),
			}
		}
		m.msi[key] = existing
		alive[key] = true
	}
	for key := range m.msi {
		if _, ok := alive[key]; !ok {
			delete(m.msi, key)
		}
	}
}

func (m *Metrics) processStatsForReplication(sourceBucket string, metrics chan<- prometheus.Metric) {
	// cbrest doesn't let us make a request to xdcr's port, so we need to do it manually
	node := m.node.RestClient().Nodes()[0]
	hostname := node.GetHostname(m.node.RestClient().AltAddr())
	scheme := "http://"
	if m.node.RestClient().TLS() {
		scheme = "https://"
	}
	url := scheme + hostname + ":" + strconv.Itoa(xdcrRestPort) + "/stats/buckets/" + sourceBucket
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		m.logger.DPanicw("Failed to create XDCR request", "url", url, "error", err)
		return
	}
	req.SetBasicAuth(m.node.Credentials())
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		m.logger.Warnw("Failed to get stats", "bucket", sourceBucket, "url", url, "error", err)
		return
	}
	defer res.Body.Close()
	payload, err := io.ReadAll(res.Body)
	if err != nil {
		m.logger.Warnw("Failed to read stats", "url", url, "error", err)
		return
	}
	var stats map[string]map[string]float64
	if err := json.Unmarshal(payload, &stats); err != nil {
		m.logger.Warnw("Failed to parse stats", "url", url, "error", err)
		return
	}
	for key, data := range stats {
		// Key will be in the format `remote_uuid/source_bucket/remote_bucket`
		// If this is a backfill pipeline, the UUID will be prefixed with `backfill_`.
		parts := strings.Split(key, "/")
		if len(parts) != 3 {
			m.logger.Warnw("Unexpected XDCR replication key", "key", key)
			continue
		}
		backfill := false
		if strings.HasPrefix(parts[0], "backfill_") {
			backfill = true
			parts[0] = strings.TrimPrefix(parts[0], "backfill_")
		}
		// labels are [targetClusterUUID, sourceBucketName, targetBucketName, pipelineType]
		labels := []string{parts[0], parts[1], parts[2], "Main"}
		if backfill {
			labels[3] = "Backfill"
		}
		for prometheusName, metric := range m.msi {
			value, ok := data[metric.Name]
			if !ok {
				m.logger.Infow("Did not find XDCR metric for requested", "prometheusName", prometheusName, "statsGroup", key, "xdcrName", metric.Name)
				continue
			}
			m.logger.Debugw("Mapped metric", "xdcrName", metric.Name, "desc", metric.desc, "type", metric.Type, "labels", labels, "value", value)
			metrics <- prometheus.MustNewConstMetric(metric.desc, metric.Type.ToPrometheus(), value, labels...)
		}
	}
}
