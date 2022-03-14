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
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHistogram(t *testing.T) {
	cases := []struct {
		Name string
		// fields are, in order: lowerBound, upperBound, count in this bin
		InputData       [][3]float64
		ExpectedBuckets map[float64]uint64
		ExpectedCount   uint64
		ExpectedSum     float64
	}{
		{
			Name: "simple",
			InputData: [][3]float64{
				{0, 10, 5},
				{10, 20, 10},
				{20, 25, 5},
			},
			ExpectedBuckets: map[float64]uint64{10: 5, 20: 15, 25: 20},
			ExpectedCount:   20,
			ExpectedSum:     287.5,
		},
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			histo := newHistogram(nil)
			for _, datum := range tc.InputData {
				histo.addReadings(datum[0], datum[1], uint64(datum[2]))
			}
			require.Equal(t, tc.ExpectedBuckets, histo.buckets, "buckets do not match")
			require.Equal(t, tc.ExpectedCount, histo.count, "count does not match")
			require.Equal(t, tc.ExpectedSum, histo.sum, "sum does not match")
		})
	}
}

func TestHistogramResample(t *testing.T) {
	cases := []struct {
		Name string
		// lowerBound, upperBound, value in bin
		InputData      [][3]float64
		ResampleBins   []float64
		ExpectedOutput map[float64]uint64
	}{
		{
			Name: "simple",
			InputData: [][3]float64{
				{0, 5, 5},
				{5, 10, 5},
				{10, 15, 5},
				{15, 20, 10},
				{20, 25, 5},
			},
			ResampleBins:   []float64{10, 20},
			ExpectedOutput: map[float64]uint64{10: 10, 20: 25, math.Inf(1): 30},
		},
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			histo := newHistogram(nil)
			for _, datum := range tc.InputData {
				histo.addReadings(datum[0], datum[1], uint64(datum[2]))
			}
			histo.resample(tc.ResampleBins)
			require.Equal(t, tc.ExpectedOutput, histo.buckets, "expected new buckets: %v\ninput data: %v", tc.ResampleBins, tc.InputData)
		})
	}
}
