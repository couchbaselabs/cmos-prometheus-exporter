package memcached

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type histogram struct {
	desc    *prometheus.Desc
	labels  []string
	buckets map[float64]uint64
	sum     float64
	count   uint64
}

func newHistogram(desc *prometheus.Desc, labels ...string) *histogram {
	return &histogram{
		desc:    desc,
		labels:  labels,
		buckets: make(map[float64]uint64),
	}
}

func (h *histogram) addReadings(lowerBound, upperBound float64, value uint64) {
	h.buckets[upperBound] = h.count + value
	h.count += value
	// This isn't very accurate, but it's the best we can do
	// https://github.com/couchbase/kv_engine/blob/f9c4a691b36bdaea82837693283af4b1b09b2a9d/include/statistics/collector.h#L283-L287
	h.sum += float64(value) * ((lowerBound + upperBound) / 2)
}

func (h *histogram) resample(newUpperBounds []float64) {
	oldUpperBounds := make([]float64, 0, len(h.buckets))
	for key := range h.buckets {
		oldUpperBounds = append(oldUpperBounds, key)
	}
	sort.Float64s(oldUpperBounds)

	sum := uint64(0)
	nextNewUpperBoundIndex := 0
	resampled := make(map[float64]uint64, len(newUpperBounds)+1)
	for i, ub := range oldUpperBounds {
		if i == 0 {
			sum += h.buckets[ub]
		} else {
			sum += h.buckets[ub] - h.buckets[oldUpperBounds[i-1]]
		}
		// If this is the last one, there's two cases: either there's a new UB that covers it, or there isn't (in which
		// case we put the remaining values into the infinity bucket).
		if i == len(oldUpperBounds)-1 {
			if nextNewUpperBoundIndex == len(newUpperBounds) {
				resampled[math.Inf(1)] = sum
			} else {
				resampled[newUpperBounds[nextNewUpperBoundIndex]] = sum
			}
			break
		}
		// If we've run out of new upper bounds, dump the remainder into infinity
		if nextNewUpperBoundIndex == len(newUpperBounds) {
			resampled[math.Inf(1)] = sum
			break
		}
		// Otherwise, check if the *next old* upper bound is above the *next new* upper bound, and transfer the current sum
		// into it (since the upper bounds are always less-than-or-equal - if we did it the other way round we'd over-count).
		if oldUpperBounds[i+1] > newUpperBounds[nextNewUpperBoundIndex] {
			resampled[newUpperBounds[nextNewUpperBoundIndex]] = sum
			nextNewUpperBoundIndex += 1
		}
	}

	h.buckets = resampled
}

func (h histogram) metric() prometheus.Metric {
	return prometheus.MustNewConstHistogram(
		h.desc,
		h.count,
		h.sum,
		h.buckets,
		h.labels...,
	)
}

func findBounds(key string) (time.Duration, time.Duration, error) {
	lastUnderscoreIdx := strings.LastIndexByte(key, '_')
	bucketBounds := key[lastUnderscoreIdx+1:]
	commaIdx := strings.IndexRune(bucketBounds, ',')
	lowerBound, err := strconv.ParseFloat(bucketBounds[:commaIdx], 64)
	if err != nil {
		return 0, 0, err
	}
	upperBound, err := strconv.ParseFloat(bucketBounds[commaIdx+1:], 64)
	return time.Duration(lowerBound) * time.Microsecond, time.Duration(upperBound) * time.Microsecond, err
}
