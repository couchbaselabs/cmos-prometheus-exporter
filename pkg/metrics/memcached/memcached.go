package memcached

import (
	"encoding/json"
	"fmt"
	"github.com/couchbase/gomemcached"
	memcached "github.com/couchbase/gomemcached/client"
	"github.com/couchbase/tools-common/cbrest"
	"github.com/markspolakovs/yacpe/pkg/couchbase"
	"github.com/markspolakovs/yacpe/pkg/metrics/common"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

type MetricConfig struct {
	// Group is the memcached stats group that this stat comes from - which may be a blank string.
	Group string `json:"group"`
	// Pattern is the regular expression to match against all stats in the memcached Group.
	Pattern string `json:"pattern"`
	// Labels are the labels to apply to the Prometheus metric.
	// `bucket`, `scope`, and `collection` are treated specially. All other values are assumed to be named
	// capturing groups in Pattern.
	Labels []string `json:"labels"`
	// ConstLabels are constant labels to apply to the metric, in addition to Labels.
	ConstLabels prometheus.Labels `json:"constLabels"`
	// Help is the help string to add to the emitted Prometheus metric.
	Help string `json:"help,omitempty"`
	// Type is the type of metric to emit (counter, gauge, untyped). Defaults to untyped.
	Type common.MetricType `json:"type"`
	// Singleton should be `true` for metrics that should only be emitted once.
	// This is necessary because the memcached protocol only allows gathering stats in the context of a bucket,
	// even for stats that are global.
	Singleton bool `json:"singleton"`
}

// MetricConfigs allows a JSON metric config to be either an object or an array.
type MetricConfigs struct {
	Values []MetricConfig
}

func (m *MetricConfigs) UnmarshalJSON(bytes []byte) error {
	switch bytes[0] {
	case '{':
		var val MetricConfig
		if err := json.Unmarshal(bytes, &val); err != nil {
			return err
		}
		m.Values = []MetricConfig{val}
		return nil
	case '[':
		return json.Unmarshal(bytes, &m.Values)
	default:
		return fmt.Errorf("invalid input for MetricConfigs")
	}
}

// MetricSet is a mapping of Prometheus metric names to MetricConfigs.
type MetricSet map[string]MetricConfigs

type internalStat struct {
	MetricConfig
	name string
	desc *prometheus.Desc
	exp  *regexp.Regexp
}

// internalStatsMap is a map of Memcached STAT groups to metrics.
type internalStatsMap map[string][]*internalStat

type Metrics struct {
	FakeCollections bool
	node            *couchbase.Node
	hostPort        string
	stats           internalStatsMap
	mc              *memcached.Client
	ms              MetricSet
	mux             sync.Mutex
	logger          *zap.Logger
}

func (m *Metrics) Describe(_ chan<- *prometheus.Desc) {
	// Don't emit any descriptors.
	// This will make this collector unchecked (see https://pkg.go.dev/github.com/prometheus/client_golang/prometheus#Collector).
	// We need to be an unchecked collector, because CB7's KV can sometimes emit metrics with inconsistent label names,
	// which a checked collector would reject. For example, it will emit kv_ops{bucket="travel-sample",op="flush"}
	// and kv_ops{bucket="travel-sample",result="hit",op="get"} simultaneously.
}

func (m *Metrics) Collect(metrics chan<- prometheus.Metric) {
	m.logger.Debug("Starting memcached collection")
	m.mux.Lock()
	defer m.mux.Unlock()
	// gomemcached doesn't have a ListBuckets method (neither does gocbcore for that matter)
	res, err := m.mc.Send(&gomemcached.MCRequest{
		Opcode: 0x87, // https://github.com/couchbase/kv_engine/blob/bb8b64eb180b01b566e2fbf54b969e6d20b2a873/docs/BinaryProtocol.md#0x87-list-buckets
	})
	if err != nil {
		m.logger.Error("When listing buckets", zap.Error(err))
	}
	buckets := strings.Split(string(res.Body), " ")
	if len(buckets) == 1 && buckets[0] == "" {
		buckets = nil
	}
	singletons := make(map[string]struct{})
	for _, bucket := range buckets {
		_, err = m.mc.SelectBucket(bucket)
		if err != nil {
			m.logger.Error("When selecting bucket", zap.String("bucket", bucket), zap.Error(err))
		}
		m.logger.Debug("Selected bucket", zap.String("bucket", bucket))
		for group := range m.stats {
			m.logger.Debug("Requesting stats for", zap.String("group", group))
			allStats, err := m.mc.StatsMap(group)
			if err != nil {
				m.logger.Error("When requesting stats map", zap.String("bucket", bucket), zap.String("group", group),
					zap.Error(err))
				continue
			}
			results, err := m.processStatGroup(bucket, group, allStats, singletons)
			if err != nil {
				m.logger.Error("When requesting stats map", zap.String("bucket", bucket), zap.String("group", group),
					zap.Error(err))
				continue
			}
			for _, metric := range results {
				metrics <- metric
			}
		}
	}
	m.logger.Debug("memcached collection done")
}

func (m *Metrics) processStatGroup(bucket string, groupName string, vals map[string]string,
	singletons map[string]struct{}) ([]prometheus.Metric,
	error) {
	result := make([]prometheus.Metric, 0, len(m.stats[groupName]))
	for _, metric := range m.stats[groupName] {
		// Skip singleton metrics we've already seen
		if metric.Singleton {
			if _, ok := singletons[metric.name]; ok {
				continue
			}
		}
		for key, valStr := range vals {
			if match := metric.exp.FindStringSubmatch(key); match != nil {
				val, err := strconv.ParseFloat(valStr, 64)
				if err != nil {
					return nil, fmt.Errorf("failed to parseFloat for stat %s (val %v): %w", key, val, err)
				}
				labelValues := make([]string, len(metric.Labels))
				for i, label := range metric.Labels {
					// Check well-known metric names
					switch label {
					case "bucket":
						labelValues[i] = bucket
					case "scope":
						fallthrough
					case "collection":
						if m.FakeCollections {
							labelValues[i] = "_default"
						} else {
							labelValues[i] = match[metric.exp.SubexpIndex(label)]
						}
					default:
						labelValues[i] = match[metric.exp.SubexpIndex(label)]
					}
				}
				result = append(result, prometheus.MustNewConstMetric(
					metric.desc,
					metric.Type.ToPrometheus(),
					val,
					labelValues...,
				))
				if metric.Singleton {
					singletons[metric.name] = struct{}{}
				}
			}
		}
	}
	return result, nil
}

func (m *Metrics) Close() error {
	return m.mc.Close()
}

func NewMemcachedMetrics(logger *zap.Logger, node *couchbase.Node, metricSet MetricSet) (*Metrics, error) {
	kvPort, err := node.GetServicePort(cbrest.ServiceData)
	if err != nil {
		return nil, err
	}
	hostPort := net.JoinHostPort(node.Hostname, strconv.Itoa(kvPort))
	logger.Debug("Connecting to", zap.String("hostPort", hostPort))
	mc, err := memcached.Connect("tcp", hostPort)
	if err != nil {
		return nil, err
	}
	_, err = mc.Auth(node.Credentials())
	if err != nil {
		return nil, err
	}
	ret := &Metrics{
		node:     node,
		mc:       mc,
		hostPort: hostPort,
		logger:   logger,
	}
	if err = ret.updateMetricSet(metricSet); err != nil {
		return nil, err
	}
	return ret, nil
}

func (m *Metrics) updateMetricSet(ms MetricSet) error {
	m.mux.Lock()
	defer m.mux.Unlock()
	// We can get away with creating a whole new stats map, including new prometheus.Desc's, because:
	// > Descriptors that share the same fully-qualified names and the same label values of their constLabels are considered equal.
	// (from https://pkg.go.dev/github.com/prometheus/client_golang/prometheus#Desc)
	statsMap := make(internalStatsMap)
	for metric, set := range ms {
		for _, val := range set.Values {
			exp, err := regexp.Compile(val.Pattern)
			if err != nil {
				return err
			}
			stat := internalStat{
				MetricConfig: val,
				name:         metric,
				exp:          exp,
				desc:         prometheus.NewDesc(metric, val.Help, val.Labels, val.ConstLabels),
			}

			// We can do this, since append(nil) will automatically make()
			statsMap[val.Group] = append(statsMap[val.Group], &stat)
		}
	}

	m.ms = ms
	m.stats = statsMap
	return nil
}
