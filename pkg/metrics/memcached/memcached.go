package memcached

import (
	"fmt"
	"github.com/couchbase/gomemcached"
	memcached "github.com/couchbase/gomemcached/client"
	"github.com/couchbase/tools-common/cbrest"
	"github.com/markspolakovs/yacpe/pkg/couchbase"
	"github.com/prometheus/client_golang/prometheus"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

type MetricConfig struct {
	Group   string   `json:"group"`
	Pattern string   `json:"pattern"`
	Labels  []string `json:"labels"`
	Help    string   `json:"help,omitempty"`
}

type MetricSet map[string]MetricConfig

type internalStat struct {
	exp    *regexp.Regexp
	labels []string
	desc   *prometheus.Desc
}

// Map of groups to metric names to metrics. IOW, stats[memcachedGroupName][prometheusMetricName]
type internalStatsMap map[string]map[string]*internalStat

type Metrics struct {
	FakeCollections bool
	node            *couchbase.Node
	hostPort        string
	stats           internalStatsMap
	mc              *memcached.Client
	ms              MetricSet
	mux             sync.Mutex
}

func (m *Metrics) Describe(descs chan<- *prometheus.Desc) {
	m.mux.Lock()
	defer m.mux.Unlock()
	for _, group := range m.stats {
		for _, stat := range group {
			descs <- stat.desc
		}
	}
	// close(descs)
}

func (m *Metrics) Collect(metrics chan<- prometheus.Metric) {
	m.mux.Lock()
	defer m.mux.Unlock()
	// gomemcached doesn't have a ListBuckets method (neither does gocbcore for that matter)
	res, err := m.mc.Send(&gomemcached.MCRequest{
		Opcode: 0x87, // https://github.com/couchbase/kv_engine/blob/bb8b64eb180b01b566e2fbf54b969e6d20b2a873/docs/BinaryProtocol.md#0x87-list-buckets
		Opaque: 0xefbeadde,
	})
	if err != nil {
		panic(err)
	}
	buckets := strings.Split(string(res.Body), " ")
	fmt.Printf("Buckets: %#v\n", buckets)
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
				fmt.Printf("Memcached sending %s\n", metric.Desc().String())
				metrics <- metric
			}
		}
	}
	// close(metrics)
	fmt.Println("memcached done")
}

func (m *Metrics) processStatGroup(bucket string, groupName string, vals map[string]string) ([]prometheus.Metric,
	error) {
	result := make([]prometheus.Metric, 0, len(m.stats[groupName]))
	for _, stat := range m.stats[groupName] {
		for key, valStr := range vals {
			if match := stat.exp.FindStringSubmatch(key); match != nil {
				val, err := strconv.ParseFloat(valStr, 64)
				if err != nil {
					return nil, fmt.Errorf("failed to parseFloat for stat %s (val %v): %w", key, val, err)
				}
				labelValues := make([]string, len(stat.labels))
				for i, label := range stat.labels {
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
							labelValues[i] = match[stat.exp.SubexpIndex(label)]
						}
					default:
						labelValues[i] = match[stat.exp.SubexpIndex(label)]
					}
				}
				result = append(result, prometheus.MustNewConstMetric(
					stat.desc,
					prometheus.GaugeValue,
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
	existing := m.stats
	if existing == nil {
		existing = make(internalStatsMap, 0)
	}
	alive := make(map[string]map[string]bool)
	for metric, val := range ms {
		if _, ok := existing[val.Group]; !ok {
			existing[val.Group] = make(map[string]*internalStat)
		}
		exp, err := regexp.Compile(val.Pattern)
		if err != nil {
			return err
		}
		result, found := existing[val.Group][metric]
		if !found {
			result = &internalStat{
				exp:    exp,
				labels: val.Labels,
				desc:   prometheus.NewDesc(metric, val.Help, val.Labels, nil),
			}
		}
		result.exp = exp
		result.labels = val.Labels
		existing[val.Group][metric] = result
		if _, ok := alive[val.Group]; !ok {
			alive[val.Group] = make(map[string]bool)
		}
		alive[val.Group][metric] = true
	}

	for group, metrics := range existing {
		for metric := range metrics {
			if _, ok := alive[group][metric]; !ok {
				delete(existing[group], metric)
			}
		}
	}

	m.ms = ms
	m.stats = existing
	return nil
}
