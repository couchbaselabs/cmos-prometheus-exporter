package common

import "github.com/prometheus/client_golang/prometheus"

type MetricType string

const (
	MetricGauge   MetricType = "gauge"
	MetricCounter MetricType = "counter"
)

func (m MetricType) ToPrometheus() prometheus.ValueType {
	switch m {
	case MetricGauge:
		return prometheus.GaugeValue
	case MetricCounter:
		return prometheus.CounterValue
	default:
		return prometheus.UntypedValue
	}
}
