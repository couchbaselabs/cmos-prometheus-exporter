package metrics

type Collector interface {
	Collect() error
}
