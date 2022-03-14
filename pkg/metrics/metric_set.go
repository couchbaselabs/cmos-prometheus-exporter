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

package metrics

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/creasty/defaults"

	"github.com/couchbaselabs/cmos-prometheus-exporter/pkg/metrics/eventing"
	"github.com/couchbaselabs/cmos-prometheus-exporter/pkg/metrics/fts"
	"github.com/couchbaselabs/cmos-prometheus-exporter/pkg/metrics/gsi"
	"github.com/couchbaselabs/cmos-prometheus-exporter/pkg/metrics/memcached"
	"github.com/couchbaselabs/cmos-prometheus-exporter/pkg/metrics/n1ql"
	"github.com/couchbaselabs/cmos-prometheus-exporter/pkg/metrics/system"
)

type MetricSet struct {
	Memcached memcached.MetricSet `json:"memcached"`
	GSI       gsi.MetricSet       `json:"gsi"`
	N1QL      n1ql.MetricSet      `json:"n1ql"`
	System    system.MetricSet    `json:"system"`
	FTS       fts.MetricSet       `json:"fts"`
	Eventing  eventing.MetricSet  `json:"eventing"`
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
