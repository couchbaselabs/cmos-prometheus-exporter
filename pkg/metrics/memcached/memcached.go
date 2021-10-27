package memcached

import (
	"encoding/json"
	"fmt"
	memcached "github.com/couchbase/gomemcached/client"
	"github.com/couchbase/tools-common/cbrest"
	"github.com/markspolakovs/yacpe/pkg/couchbase"
	"github.com/prometheus/client_golang/prometheus"
	"log"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"sync"
)

type MetricConfig struct {
	Group string `json:"group"`
	Pattern string `json:"pattern"`
	Labels []string `json:"labels"`
}

type MetricSet map[string]MetricConfig


type metricInternal struct {
	exp *regexp.Regexp
	labels []string
	gauge *prometheus.GaugeVec
}

// Map of groups to metric names to metrics. IOW, stats[memcachedGroupName][prometheusMetricName]
type internalStatsMap map[string]map[string]*metricInternal

type Metrics struct {
	node *couchbase.Node
	hostPort string
	stats internalStatsMap
	mc  *memcached.Client
	ms  MetricSet
	mux sync.Mutex
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
		node: node,
		mc:    mc,
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
			existing[val.Group] = make(map[string]*metricInternal)
		}
		exp, err := regexp.Compile(val.Pattern)
		if err != nil {return err}
		result, found := existing[val.Group][metric]
		if !found {
			result = &metricInternal{
				gauge:  prometheus.NewGaugeVec(prometheus.GaugeOpts{
					Name:        metric,
				}, val.Labels),
			}
			prometheus.MustRegister(result.gauge)
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
		for metric, val := range metrics {
			if _, ok := alive[group][metric]; !ok {
				// dead, remove it
				prometheus.Unregister(val.gauge)
				delete(existing[group], metric)
			}
		}
	}

	m.ms = ms
	m.stats = existing
	return nil
}

func (m *Metrics) processStatGroup(bucket string, groupName string, vals map[string]string) error {
	for _, stat := range m.stats[groupName] {
		for key, valStr := range vals {
			if match := stat.exp.FindStringSubmatch(key); match != nil {
				val, err := strconv.ParseFloat(valStr, 64)
				if err != nil {
					return fmt.Errorf("failed to parseFloat for stat %s (val %v): %w", key, val, err)
				}
				labelValues := make([]string, len(stat.labels))
				for i, label := range stat.labels {
					// Check well-known metric names
					switch label {
					case "bucket":
						labelValues[i] = bucket
					default:
						labelValues[i] = match[stat.exp.SubexpIndex(label)]
					}
				}
				stat.gauge.WithLabelValues(labelValues...).Set(val)
			}
		}
	}
	return nil
}

func (m *Metrics) Collect() error {
	m.mux.Lock()
	defer m.mux.Unlock()
	res, err := m.node.RestClient().Execute(&cbrest.Request{
		Method:   http.MethodGet,
		Endpoint: cbrest.EndpointBuckets,
		Service:  cbrest.ServiceManagement,
		ExpectedStatusCode: http.StatusOK,
	})
	if err != nil {
		return err
	}
	type bucketsInfo []struct{
		Name string `json:"name"`
		VBucketServerMap       struct {
			ServerList    []string `json:"serverList"`
		} `json:"vBucketServerMap"`
	}
	var buckets bucketsInfo
	if err = json.Unmarshal(res.Body, &buckets); err != nil {
		return err
	}
	ourIP := net.ParseIP(m.node.Hostname)
	// Special case
	if ourIP == nil && m.node.Hostname == "localhost" {
		ourIP = net.ParseIP("127.0.0.1")
	}
	for _, bucket := range buckets {
		var weHaveIt bool
		for _, hostPort := range bucket.VBucketServerMap.ServerList {
			weHaveIt = sameHost(hostPort, m.hostPort)
			if weHaveIt {
				break
			}
		}
		if weHaveIt {
			_, err := m.mc.SelectBucket(bucket.Name)
			if err != nil {
				return err
			}
			for group := range m.stats {
				allStats, err := m.mc.StatsMap(group)
				if err != nil {
					return fmt.Errorf("while executing memcached stats for %v: %w", group, err)
				}
				if err := m.processStatGroup(bucket.Name, group, allStats); err != nil {
					return fmt.Errorf("while processing group %v: %w", group, err)
				}
			}
		}
	}
	return nil
}

const localhostIP = "127.0.0.1"

func sameHost(theirHostPort string, ourHostPort string) bool {
	theirHost, theirPort, err := net.SplitHostPort(theirHostPort)
	if err != nil {
		log.Printf("The REST API gave us a duff theirHostPort %s: %v", theirHostPort, err)
		return false
	}
	ourHost, ourPort, err := net.SplitHostPort(ourHostPort)
	if err != nil {
		panic(fmt.Errorf("our hostPort is duff (%s): %w", ourHostPort, err))
	}
	if theirHost == ourHost && theirPort == ourPort {
		return true
	}
	theirIP := net.ParseIP(theirHost)
	if theirIP == nil && theirHost == "localhost" {
		theirIP = net.ParseIP(localhostIP)
	}
	ourIP := net.ParseIP(ourHost)
	if ourIP == nil && ourHost == "localhost" {
		ourIP = net.ParseIP(localhostIP)
	}
	if theirIP != nil && ourIP != nil {
		if ourIP.Equal(theirIP) {
			return theirPort == ourPort
		}
		if ourIP.IsLoopback() && theirIP.IsLoopback() {
			return theirPort == ourPort
		}
	}
	return false
}
