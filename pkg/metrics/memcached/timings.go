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
	"sort"

	"github.com/couchbase/gomemcached"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// Values are [upper bound, ops, percentile]
type commandTimings [][3]float64

type commandTimingsResponse struct {
	BucketsLow float64        `json:"bucketsLow"`
	Data       commandTimings `json:"data"`
	Total      float64        `json:"total"`
}

func (m *Metrics) processCommandTimings(metrics chan<- prometheus.Metric, bucket string) error {
	for _, opcode := range m.commandTimings.Opcodes {
		m.logger.Debug("Requesting command timings", zap.String("key", bucket), zap.String("opcode", opcode.name))
		res, err := m.mc.Send(&gomemcached.MCRequest{
			Opcode: 0xf3,
			Key:    []byte(bucket),
			Keylen: len(bucket),
			Extras: []byte{byte(opcode.code)},
			Opaque: 374593, // chosen by fair dice roll, guaranteed to be random
		})
		if err != nil {
			return fmt.Errorf("failed to get command timings for opcode %s: %w", opcode.name, err)
		}
		var data commandTimingsResponse
		if err := json.Unmarshal(res.Body, &data); err != nil {
			return fmt.Errorf("failed to unmarshal command timings for opcode %s: %w", opcode.name, err)
		}

		if len(data.Data) == 0 {
			m.logger.Debug("Zero command timings, omitting histogram", zap.String("opcode", opcode.name))
			continue
		}

		sort.SliceStable(data.Data, func(i, j int) bool {
			return data.Data[i][0] < data.Data[j][0]
		})

		histo := newHistogram(m.commandTimings.desc, bucket, opcode.name)

		lastUpperBound := data.BucketsLow
		for _, datum := range data.Data {
			// We can sometimes get bins with a zero value, because KV will insert bins at every 5th percentile
			// even if there's no data there. We can safely skip them.
			if datum[1] == 0 {
				continue
			}
			upperBound := datum[0] / 1e+6 // convert us to s
			count := uint64(datum[1])
			// m.logger.Debug("Adding timings", zap.String("opcode", opcode.name), zap.Float64("upperBound", upperBound), zap.Uint64("count", count), zap.Float64("lUB", lastUpperBound))
			histo.addReadings(lastUpperBound, upperBound, count)
			lastUpperBound = upperBound
		}
		if m.commandTimings.ResampleBuckets != nil {
			histo.resample(m.commandTimings.ResampleBuckets)
		}
		metrics <- histo.metric()
	}
	return nil
}
