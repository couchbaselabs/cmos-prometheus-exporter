package metrics

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/creasty/defaults"
	"github.com/markspolakovs/yacpe/pkg/metrics/gsi"
	"github.com/markspolakovs/yacpe/pkg/metrics/memcached"
)

type MetricSet struct {
	Memcached memcached.MetricSet `json:"memcached"`
	GSI       gsi.MetricSet       `json:"gsi"`
}

//go:embed defaultMetricSet.json
var defaultMetricSet []byte

func LoadDefaultMetricSet() *MetricSet {
	ms, err := ParseMetricSet(defaultMetricSet)
	if err != nil {
		panic(fmt.Errorf("failed to load default metric set: %w", err))
	}
	return ms
}

func ParseMetricSet(val []byte) (*MetricSet, error) {
	var ms MetricSet
	err := json.Unmarshal(val, &ms)
	if err != nil {
		return nil, err
	}
	err = defaults.Set(&ms)
	return &ms, err
}
