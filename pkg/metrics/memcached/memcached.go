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
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

type MetricConfig struct {
	Group       string            `json:"group"`
	Pattern     string            `json:"pattern"`
	Labels      []string          `json:"labels"`
	ConstLabels prometheus.Labels `json:"constLabels"`
	Help        string            `json:"help,omitempty"`
	Type        common.MetricType `json:"type" default:"gauge"`
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

type MetricSet map[string]MetricConfigs

type internalStat struct {
	desc      *prometheus.Desc
	exp       *regexp.Regexp
	valueType common.MetricType
	labels    []string
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
}

func (m *Metrics) Describe(_ chan<- *prometheus.Desc) {
	// Don't emit any descriptors.
	// This will make this collector unchecked (see https://pkg.go.dev/github.com/prometheus/client_golang/prometheus#Collector).
	// We need to be an unchecked collector, because CB7's KV can sometimes emit metrics with inconsistent label names,
	// which a checked collector would reject. For example, it will emit kv_ops{bucket="travel-sample",op="flush"}
	// and kv_ops{bucket="travel-sample",result="hit",op="get"} simultaneously.
}

func (m *Metrics) Collect(metrics chan<- prometheus.Metric) {
	m.mux.Lock()
	defer m.mux.Unlock()
	// gomemcached doesn't have a ListBuckets method (neither does gocbcore for that matter)
	res, err := m.mc.Send(&gomemcached.MCRequest{
		Opcode: 0x87, // https://github.com/couchbase/kv_engine/blob/bb8b64eb180b01b566e2fbf54b969e6d20b2a873/docs/BinaryProtocol.md#0x87-list-buckets
	})
	if err != nil {
		panic(err)
	}
	buckets := strings.Split(string(res.Body), " ")
	if len(buckets) == 1 && buckets[0] == "" {
		buckets = nil
	}
	for _, bucket := range buckets {
		_, err = m.mc.SelectBucket(bucket)
		if err != nil {
			panic(err)
		}
		for group := range m.stats {
			allStats, err := m.mc.StatsMap(group)
			if err != nil {
				panic(fmt.Errorf("while executing memcached stats for %v: %w", group, err))
			}
			results, err := m.processStatGroup(bucket, group, allStats)
			if err != nil {
				panic(fmt.Errorf("while processing group %v: %w", group, err))
			}
			for _, metric := range results {
				metrics <- metric
			}
		}
	}
}

func (m *Metrics) processStatGroup(bucket string, groupName string, vals map[string]string) ([]prometheus.Metric,
	error) {
	result := make([]prometheus.Metric, 0, len(m.stats[groupName]))
	for _, metric := range m.stats[groupName] {
		for key, valStr := range vals {
			if match := metric.exp.FindStringSubmatch(key); match != nil {
				val, err := strconv.ParseFloat(valStr, 64)
				if err != nil {
					return nil, fmt.Errorf("failed to parseFloat for stat %s (val %v): %w", key, val, err)
				}
				labelValues := make([]string, len(metric.labels))
				for i, label := range metric.labels {
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
					metric.valueType.ToPrometheus(),
					val,
					labelValues...,
				))
			}
		}
	}
	return result, nil
}

func (m *Metrics) Close() error {
	return m.mc.Close()
}

func NewMemcachedMetrics(node *couchbase.Node, metricSet MetricSet) (*Metrics, error) {
	kvPort, err := node.GetServicePort(cbrest.ServiceData)
	if err != nil {
		return nil, err
	}
	hostPort := net.JoinHostPort(node.Hostname, strconv.Itoa(kvPort))
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
				exp:       exp,
				labels:    val.Labels,
				valueType: val.Type,
				desc:      prometheus.NewDesc(metric, val.Help, val.Labels, val.ConstLabels),
			}

			// We can do this, since append(nil) will automatically make()
			statsMap[val.Group] = append(statsMap[val.Group], &stat)
		}
	}

	m.ms = ms
	m.stats = statsMap
	return nil
}
