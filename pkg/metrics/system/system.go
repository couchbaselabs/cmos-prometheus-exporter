package system

import (
	"runtime"
	"time"

	"github.com/cloudfoundry/gosigar"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

type MetricName string

const (
	MemFree       MetricName = "memFree"
	MemTotal                 = "memTotal"
	MemActualFree            = "memActualFree"
	MemActualUsed            = "memActualUsed"
	// TODO
	//MemUsedSys                   = "memUsedSys"
	cpuUtilization    = "cpuUtilization"
	cpuUser           = "cpuUser"
	cpuSys            = "cpuSys"
	cpuIrq            = "cpuIrq"
	cpuStolen         = "cpuStolen"
	cpuCoresAvailable = "cpuCoresAvailable"
)

var metricLabels = map[MetricName][]string{
	MemFree:  {},
	MemTotal: {},
}

type Metric struct {
	Name        string            `json:"name"`
	Help        string            `json:"help"`
	ConstLabels prometheus.Labels `json:"labels"`
	desc        *prometheus.Desc
}

// MetricSet is the metrics used by the system collector.
//
// NOTE: this works differently to the other collectors - the keys are well-known
type MetricSet map[MetricName]*Metric

type Collector struct {
	logger *zap.SugaredLogger
	sigar  *sigar.ConcreteSigar
	ms     MetricSet
}

func NewSystemMetrics(logger *zap.SugaredLogger, ms MetricSet) *Collector {
	return &Collector{
		logger: logger,
		ms:     ms,
		sigar:  new(sigar.ConcreteSigar),
	}
}

func (c *Collector) Describe(descs chan<- *prometheus.Desc) {
	c.prepareMetrics()
	for _, metric := range c.ms {
		if metric.desc != nil {
			descs <- metric.desc
		}
	}
}

func (c *Collector) Collect(metrics chan<- prometheus.Metric) {
	c.memMetrics(metrics)
	c.cpuMetrics(metrics)
}

func (c *Collector) memMetrics(metrics chan<- prometheus.Metric) {
	// Alas, for consistency with CB we need to ignore cgroups
	mem, err := c.sigar.GetMemIgnoringCGroups()
	if err != nil {
		c.logger.Errorw("Failed to collect memory stats", "error", err)
		return
	}
	if m, ok := c.ms[MemFree]; ok {
		metrics <- prometheus.MustNewConstMetric(m.desc, prometheus.UntypedValue, float64(mem.Free))
	}
	if m, ok := c.ms[MemTotal]; ok {
		metrics <- prometheus.MustNewConstMetric(m.desc, prometheus.UntypedValue, float64(mem.Total))
	}
	if m, ok := c.ms[MemActualFree]; ok {
		metrics <- prometheus.MustNewConstMetric(m.desc, prometheus.UntypedValue, float64(mem.ActualFree))
	}
	if m, ok := c.ms[MemActualUsed]; ok {
		metrics <- prometheus.MustNewConstMetric(m.desc, prometheus.UntypedValue, float64(mem.ActualUsed))
	}
}

func (c *Collector) prepareMetrics() {
	for key, metric := range c.ms {
		if metric.Name == "" {
			continue
		}
		if metric.desc == nil {
			c.ms[key].desc = prometheus.NewDesc(metric.Name, metric.Help, metricLabels[key], metric.ConstLabels)
		}
	}
}

func (c *Collector) cpuMetrics(metrics chan<- prometheus.Metric) {
	cpuCh, stop := c.sigar.CollectCpuStats(time.Second)
	// we only care about the first
	close(stop)
	cpu := <-cpuCh
	if m, ok := c.ms[cpuUtilization]; ok {
		metrics <- prometheus.MustNewConstMetric(m.desc, prometheus.UntypedValue, float64(cpu.Total()))
	}
	if m, ok := c.ms[cpuUser]; ok {
		metrics <- prometheus.MustNewConstMetric(m.desc, prometheus.UntypedValue, float64(cpu.User))
	}
	if m, ok := c.ms[cpuSys]; ok {
		metrics <- prometheus.MustNewConstMetric(m.desc, prometheus.UntypedValue, float64(cpu.Sys))
	}
	if m, ok := c.ms[cpuIrq]; ok {
		metrics <- prometheus.MustNewConstMetric(m.desc, prometheus.UntypedValue, float64(cpu.Irq))
	}
	if m, ok := c.ms[cpuStolen]; ok {
		metrics <- prometheus.MustNewConstMetric(m.desc, prometheus.UntypedValue, float64(cpu.Stolen))
	}
	if m, ok := c.ms[cpuCoresAvailable]; ok {
		metrics <- prometheus.MustNewConstMetric(m.desc, prometheus.UntypedValue, float64(runtime.NumCPU()))
	}
}
