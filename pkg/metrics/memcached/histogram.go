package memcached

import (
	"github.com/prometheus/client_golang/prometheus"
	"math"
	"sort"
)

type histogram struct {
	im      *internalStat
	labels  []string
	buckets map[float64]uint64
	sum     float64
	count   uint64
}

func (h *histogram) addReadings(lowerBound, upperBound float64, value uint64) {
	h.buckets[upperBound] = h.count + value
	h.count += value
	h.sum += float64(value) * (upperBound - lowerBound)
}

func (h *histogram) resample(newBuckets []float64) {
	upperBounds := make([]float64, len(h.buckets))
	i := 0
	for key := range h.buckets {
		upperBounds[i] = key
		i++
	}
	sort.Float64s(upperBounds)

	sum := uint64(0)
	newBucketIndex := 0
	resampled := make(map[float64]uint64, len(newBuckets)+1)
	for i := 0; i < len(upperBounds); i++ {
		ub := upperBounds[i]
		val := h.buckets[ub]
		if newBucketIndex == len(newBuckets) {
			// Deal with any remaining values and flush them into the infinity bucket
			sum += val
			continue
		}
		currentNewUpperBound := newBuckets[newBucketIndex]
		if ub > currentNewUpperBound {
			// Flush the current sum into resampled
			resampled[currentNewUpperBound] = sum
			newBucketIndex++
			sum = val
			continue
		}
		sum += val
	}
	resampled[math.Inf(1)] = sum

	h.buckets = resampled
}

func (h histogram) metric() prometheus.Metric {
	return prometheus.MustNewConstHistogram(
		h.im.desc,
		h.count,
		h.sum,
		h.buckets,
		h.labels...,
	)
}
