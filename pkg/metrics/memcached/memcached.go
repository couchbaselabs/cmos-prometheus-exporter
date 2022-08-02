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

package memcached

import (
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/couchbase/gomemcached"
	memcached "github.com/couchbase/gomemcached/client"
	"github.com/couchbase/tools-common/cbrest"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/atomic"
	"go.uber.org/zap"

	"github.com/couchbaselabs/cmos-prometheus-exporter/pkg/couchbase"
	"github.com/couchbaselabs/cmos-prometheus-exporter/pkg/metrics/common"
)

func init() {
	// gomemcached doesn't have names for these commands
	gomemcached.CommandNames[gomemcached.GET_META] = "GET_META"                 //nolint:nosnakecase
	gomemcached.CommandNames[gomemcached.SET_WITH_META] = "SET_WITH_META"       //nolint:nosnakecase
	gomemcached.CommandNames[gomemcached.ADD_WITH_META] = "ADD_WITH_META"       //nolint:nosnakecase
	gomemcached.CommandNames[gomemcached.DELETE_WITH_META] = "DELETE_WITH_META" //nolint:nosnakecase
	gomemcached.CommandNames[gomemcached.SET_VBUCKET] = "SET_VBUCKET"           //nolint:nosnakecase
}

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
	// Type is the type of metric to emit (counter, gauge, histogram, untyped). Defaults to untyped.
	Type common.MetricType `json:"type"`
	// Multiplier is a constant by which to multiply the resulting stats value.
	// For example, it can be used to fix values that are milliseconds in 6.0 but seconds in 7.0,
	// by setting a multiplier of 0.001.
	// If unset, defaults to 1 (no change).
	// NOTE: for histograms, the multiplier is applied to the *bounds*, not the values.
	Multiplier float64 `json:"multiplier"`
	// Singleton should be `true` for metrics that should only be emitted once.
	// This is necessary because the memcached protocol only allows gathering stats in the context of a bucket,
	// even for stats that are global.
	Singleton bool `json:"singleton"`
	// ResampleBuckets is only applicable for histograms.
	// It represents the bucket values that the original memcached buckets should be remapped to.
	ResampleBuckets []float64 `json:"resampleBuckets"`
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

type commandTimingMetricConfig struct {
	Opcodes         []mcOpcode `json:"opcodes"`
	ResampleBuckets []float64  `json:"resampleBuckets"`
	desc            *prometheus.Desc
}

// MetricSet is a mapping of Prometheus metric names to MetricConfigs.
type MetricSet struct {
	Stats map[string]MetricConfigs `json:"stats"`
	// CommandTimings is the configuration for generating the kv_cmd_duration_seconds histogram.
	// It is a special case, as it requires different processing to the other memcached stats.
	CommandTimings *commandTimingMetricConfig `json:"commandTimings"`
}

type internalStat struct {
	MetricConfig
	name       string
	desc       *prometheus.Desc
	exp        *regexp.Regexp
	multiplier float64
}

// internalStatsMap is a map of Memcached STAT groups to metrics.
type internalStatsMap map[string][]*internalStat

type Metrics struct {
	FakeCollections bool
	node            couchbase.NodeCommon
	hostPort        string
	stats           internalStatsMap
	commandTimings  *commandTimingMetricConfig
	mc              *memcached.Client
	ms              MetricSet
	mux             sync.Mutex
	logger          *zap.Logger
	opaqueInc       *atomic.Uint32
}

func (m *Metrics) Describe(_ chan<- *prometheus.Desc) {
	// Don't emit any descriptors.
	// This will make this collector unchecked (see https://pkg.go.dev/github.com/prometheus/client_golang/prometheus#Collector).
	// We need to be an unchecked collector, because CB7's KV can sometimes emit metrics with inconsistent label names,
	// which a checked collector would reject. For example, it will emit kv_ops{bucket="travel-sample",op="flush"}
	// and kv_ops{bucket="travel-sample",result="hit",op="get"} simultaneously.
}

func (m *Metrics) Collect(metrics chan<- prometheus.Metric) {
	start := time.Now()
	defer func() {
		end := time.Now()
		m.logger.Info("Completed memcached collection", zap.Duration("elapsed", end.Sub(start)))
	}()
	m.logger.Info("Starting memcached collection")
	m.mux.Lock()
	defer m.mux.Unlock()
	// gomemcached doesn't have a ListBuckets method (neither does gocbcore for that matter)
	res, err := m.mc.Send(&gomemcached.MCRequest{
		Opcode: 0x87, // https://github.com/couchbase/kv_engine/blob/bb8b64eb180b01b566e2fbf54b969e6d20b2a873/docs/BinaryProtocol.md#0x87-list-buckets
	})
	if err != nil {
		m.logger.Error("When listing buckets", zap.Error(err))
		return
	}
	if res == nil {
		m.logger.Error("Memcached gave nil response to ListBuckets")
		return
	}
	buckets := strings.Split(string(res.Body), " ")
	if len(buckets) == 1 && buckets[0] == "" {
		buckets = nil
	}
	m.logger.Debug("Got buckets", zap.Strings("buckets", buckets))
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
			if err := m.processStatGroup(metrics, bucket, group, allStats, singletons); err != nil {
				m.logger.Error("When requesting stats map", zap.String("bucket", bucket), zap.String("group", group),
					zap.Error(err))
				continue
			}
		}

		if m.commandTimings != nil {
			if err := m.processCommandTimings(metrics, bucket); err != nil {
				m.logger.Error("Failed to process command timings", zap.String("bucket", bucket), zap.Error(err))
			}
		}
	}
}

func (m *Metrics) processStatGroup(metrics chan<- prometheus.Metric, bucket string, groupName string, vals map[string]string,
	singletons map[string]struct{},
) error {
	for _, metric := range m.stats[groupName] {
		// Skip singleton metrics we've already seen
		if metric.Singleton {
			if _, ok := singletons[metric.name]; ok {
				continue
			}
		}
		var err error
		switch metric.Type {
		case common.MetricHistogram:
			err = m.mapHistogramStat(metrics, bucket, vals, metric)
		default:
			err = m.mapValueStat(metrics, bucket, vals, metric)
		}
		if err != nil {
			m.logger.Warn("Failed to process stat", zap.String("metric", metric.name), zap.Error(err))
			continue
		}
		if metric.Singleton {
			singletons[metric.name] = struct{}{}
		}
	}
	return nil
}

func (m *Metrics) mapValueStat(metrics chan<- prometheus.Metric, bucket string, statsValues map[string]string,
	metric *internalStat,
) error {
	for key, valStr := range statsValues {
		if match := metric.exp.FindStringSubmatch(key); match != nil {
			val, err := strconv.ParseFloat(valStr, 64)
			if err != nil {
				return fmt.Errorf("failed to parseFloat for stat %s (val %v): %w", key, val, err)
			}
			labelValues := m.resolveLabelValues(bucket, metric, match)
			// m.logger.Debug("Mapped metric", zap.String("memcached_name", key), zap.String("prom_name", metric.name),
			//	zap.Strings("labels", labelValues))
			metrics <- prometheus.MustNewConstMetric(
				metric.desc,
				metric.Type.ToPrometheus(),
				val*metric.multiplier,
				labelValues...,
			)
		}
	}
	return nil
}

func (m *Metrics) mapHistogramStat(metrics chan<- prometheus.Metric, bucket string, vals map[string]string,
	metric *internalStat,
) error {
	matchedKeys := make([]string, 0)
	for key := range vals {
		if metric.exp.MatchString(key) {
			matchedKeys = append(matchedKeys, key)
		}
	}
	if len(matchedKeys) == 0 {
		return nil
	}

	sort.Slice(matchedKeys, func(i, j int) bool {
		lbI, _, err := findBounds(matchedKeys[i])
		if err != nil {
			panic(err)
		}
		lbJ, _, err := findBounds(matchedKeys[j])
		if err != nil {
			panic(err)
		}
		return lbI < lbJ
	})

	histograms := make(map[string]*histogram)
	for _, key := range matchedKeys {
		lastUnderscoreIdx := strings.LastIndexByte(key, '_')
		statName := key[:lastUnderscoreIdx]
		histo, ok := histograms[statName]
		if !ok {
			histo = newHistogram(metric.desc, m.resolveLabelValues(bucket, metric, metric.exp.FindStringSubmatch(key))...)
			histograms[statName] = histo
		}
		lowerBound, upperBound, err := findBounds(key)
		if err != nil {
			return err
		}
		val, err := strconv.ParseUint(vals[key], 10, 64)
		if err != nil {
			return err
		}
		histo.addReadings(lowerBound.Seconds()*metric.Multiplier, upperBound.Seconds()*metric.Multiplier, val)
	}

	for _, histo := range histograms {
		if len(metric.ResampleBuckets) > 0 {
			histo.resample(metric.ResampleBuckets)
		}
		metrics <- histo.metric()
	}
	return nil
}

func (m *Metrics) resolveLabelValues(bucket string, metric *internalStat, match []string) []string {
	labelValues := make([]string, len(metric.Labels))
	for i, label := range metric.Labels {
		transformFn := func(s string) string {
			return s
		}
		if strings.ContainsRune(label, ':') {
			parts := strings.SplitN(label, ":", 2)
			label = parts[0]
			switch parts[1] {
			case "uppercase":
				transformFn = strings.ToUpper
			case "lowercase":
				transformFn = strings.ToLower
			default:
				m.logger.DPanic("Unknown label transformer",
					zap.String("transform", parts[1]),
					zap.String("label", label))
			}
		}
		// Check well-known metric names
		switch label {
		case "bucket":
			labelValues[i] = transformFn(bucket)
		case "scope":
			fallthrough
		case "collection":
			if m.FakeCollections {
				labelValues[i] = transformFn("_default")
			} else if metric.exp.SubexpIndex(label) > 0 {
				labelValues[i] = transformFn(match[metric.exp.SubexpIndex(label)])
			}
		default:
			if metric.exp.SubexpIndex(label) == -1 {
				m.logger.Warn("Missing sub-expression for label match", zap.String("label", label), zap.Strings("match", match), zap.String("metric", metric.name))
			}
			labelValues[i] = transformFn(match[metric.exp.SubexpIndex(label)])
		}
	}
	return labelValues
}

func (m *Metrics) Close() error {
	return m.mc.Close()
}

func NewMemcachedMetrics(logger *zap.Logger, node couchbase.NodeCommon, metricSet MetricSet) (*Metrics, error) {
	kvPort, err := node.GetServicePort(cbrest.ServiceData)
	if err != nil {
		return nil, err
	}
	hostPort := net.JoinHostPort(node.Hostname(), strconv.Itoa(kvPort))
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
		node:      node,
		mc:        mc,
		hostPort:  hostPort,
		logger:    logger,
		opaqueInc: atomic.NewUint32(0),
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
	for metric, set := range ms.Stats {
		for _, val := range set.Values {
			exp, err := regexp.Compile(val.Pattern)
			if err != nil {
				return err
			}
			labels := make([]string, len(val.Labels))
			for i, label := range val.Labels {
				if strings.ContainsRune(label, ':') {
					parts := strings.SplitN(label, ":", 2)
					labels[i] = parts[0]
				} else {
					labels[i] = label
				}
			}
			multiplier := val.Multiplier
			if multiplier == 0 {
				multiplier = 1
			}
			stat := internalStat{
				MetricConfig: val,
				name:         metric,
				exp:          exp,
				desc:         prometheus.NewDesc(metric, val.Help, labels, val.ConstLabels),
				multiplier:   multiplier,
			}

			// We can do this, since append(nil) will automatically make()
			statsMap[val.Group] = append(statsMap[val.Group], &stat)
		}
	}

	m.ms = ms
	m.stats = statsMap
	if ms.CommandTimings != nil {
		m.commandTimings = &*ms.CommandTimings
		m.commandTimings.desc = prometheus.NewDesc("kv_cmd_duration_seconds", "command durations", []string{
			"bucket",
			"opcode",
		}, nil)
	} else {
		m.commandTimings = nil
	}
	return nil
}
