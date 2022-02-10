package metrics

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/creasty/defaults"

	"github.com/markspolakovs/yacpe/pkg/metrics/fts"
	"github.com/markspolakovs/yacpe/pkg/metrics/gsi"
	"github.com/markspolakovs/yacpe/pkg/metrics/memcached"
	"github.com/markspolakovs/yacpe/pkg/metrics/n1ql"
	"github.com/markspolakovs/yacpe/pkg/metrics/system"
)

type MetricSet struct {
	Memcached memcached.MetricSet `json:"memcached"`
	GSI       gsi.MetricSet       `json:"gsi"`
	N1QL      n1ql.MetricSet      `json:"n1ql"`
	System    system.MetricSet    `json:"system"`
	FTS       fts.MetricSet       `json:"fts"`
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
