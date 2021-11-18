package common

import "github.com/prometheus/client_golang/prometheus"

type MetricType string

const (
	MetricGauge     MetricType = "gauge"
	MetricCounter   MetricType = "counter"
	MetricHistogram MetricType = "histogram"
)

func (m MetricType) ToPrometheus() prometheus.ValueType {
	switch m {
	case MetricGauge:
		return prometheus.GaugeValue
	case MetricCounter:
		return prometheus.CounterValue
	case MetricHistogram:
		panic("Histogram can't be converted into a ValueType")
	default:
		return prometheus.UntypedValue
	}
}
