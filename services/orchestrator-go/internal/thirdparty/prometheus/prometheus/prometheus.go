package prometheus

import (
	"math"
	"sort"
	"strings"
	"sync"
)

// MetricType represents the type of metric collected.
type MetricType string

const (
	// CounterValue represents monotonically increasing counters.
	CounterValue MetricType = "counter"
	// HistogramValue represents histogram metrics.
	HistogramValue MetricType = "histogram"
)

// MetricFamily models a Prometheus metric family.
type MetricFamily struct {
	Name    string
	Help    string
	Type    MetricType
	Metrics []Metric
}

// Metric holds the sampled values for a metric label set.
type Metric struct {
	Labels  map[string]string
	Value   float64
	Buckets []Bucket
	Sum     float64
	Count   uint64
}

// Bucket captures a histogram bucket count.
type Bucket struct {
	UpperBound float64
	Count      uint64
}

// Collector represents a metric collector registered with the registry.
type Collector interface {
	metricFamilies() []*MetricFamily
}

// Registerer registers collectors with a registry.
type Registerer interface {
	MustRegister(...Collector)
}

// Gatherer gathers metric families.
type Gatherer interface {
	Gather() ([]*MetricFamily, error)
}

// Registry stores collectors and exposes gather operations.
type Registry struct {
	mu         sync.Mutex
	collectors []Collector
}

// NewRegistry constructs an empty registry instance.
func NewRegistry() *Registry {
	return &Registry{collectors: []Collector{}}
}

// MustRegister registers the provided collectors.
func (r *Registry) MustRegister(cs ...Collector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.collectors = append(r.collectors, cs...)
}

// Gather collects all metric families from registered collectors.
func (r *Registry) Gather() ([]*MetricFamily, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	families := make([]*MetricFamily, 0, len(r.collectors))
	for _, collector := range r.collectors {
		if collector == nil {
			continue
		}
		families = append(families, collector.metricFamilies()...)
	}
	return families, nil
}

var defaultRegistry = NewRegistry()

// DefaultRegisterer is the package-level registerer used when none is provided.
var DefaultRegisterer Registerer = defaultRegistry

// DefaultGatherer is the package-level gatherer exposed via promhttp.
var DefaultGatherer Gatherer = defaultRegistry

// HistogramOpts describes a histogram metric.
type HistogramOpts struct {
	Name    string
	Help    string
	Buckets []float64
}

// CounterOpts describes a counter metric.
type CounterOpts struct {
	Name string
	Help string
}

// Observer records floating point observations.
type Observer interface {
	Observe(float64)
}

// Counter is a monotonically increasing value.
type Counter interface {
	Inc()
	Add(float64)
}

// DefBuckets mirrors the default Prometheus histogram buckets.
var DefBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

type Histogram struct {
	opts         HistogramOpts
	buckets      []float64
	bucketCounts []uint64
	sum          float64
	count        uint64
	labels       map[string]string
	mu           sync.Mutex
}

// NewHistogram constructs a histogram collector.
func NewHistogram(opts HistogramOpts) *Histogram {
	return newHistogram(opts, nil)
}

func newHistogram(opts HistogramOpts, labels map[string]string) *Histogram {
	buckets := append([]float64(nil), opts.Buckets...)
	if len(buckets) == 0 {
		buckets = append([]float64(nil), DefBuckets...)
	}
	sort.Float64s(buckets)
	copiedLabels := make(map[string]string, len(labels))
	for k, v := range labels {
		copiedLabels[k] = v
	}
	return &Histogram{
		opts:         opts,
		buckets:      buckets,
		bucketCounts: make([]uint64, len(buckets)+1),
		labels:       copiedLabels,
	}
}

// Observe records a sample in the histogram.
func (h *Histogram) Observe(value float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sum += value
	h.count++
	idx := len(h.bucketCounts) - 1
	for i, bound := range h.buckets {
		if value <= bound {
			idx = i
			break
		}
	}
	h.bucketCounts[idx]++
}

func (h *Histogram) metricFamilies() []*MetricFamily {
	return []*MetricFamily{{
		Name:    h.opts.Name,
		Help:    h.opts.Help,
		Type:    HistogramValue,
		Metrics: []Metric{h.snapshot()},
	}}
}

func (h *Histogram) snapshot() Metric {
	h.mu.Lock()
	defer h.mu.Unlock()
	cumulative := make([]Bucket, len(h.bucketCounts))
	var running uint64
	for i := 0; i < len(h.bucketCounts); i++ {
		running += h.bucketCounts[i]
		upper := math.Inf(1)
		if i < len(h.buckets) {
			upper = h.buckets[i]
		}
		cumulative[i] = Bucket{UpperBound: upper, Count: running}
	}
	labels := make(map[string]string, len(h.labels))
	for k, v := range h.labels {
		labels[k] = v
	}
	return Metric{
		Labels:  labels,
		Buckets: cumulative,
		Sum:     h.sum,
		Count:   h.count,
	}
}

type HistogramVec struct {
	opts       HistogramOpts
	labelNames []string
	mu         sync.Mutex
	metrics    map[string]*Histogram
}

// NewHistogramVec constructs a histogram vector collector.
func NewHistogramVec(opts HistogramOpts, labelNames []string) *HistogramVec {
	return &HistogramVec{
		opts:       opts,
		labelNames: append([]string(nil), labelNames...),
		metrics:    make(map[string]*Histogram),
	}
}

// WithLabelValues retrieves the histogram for the provided label values.
func (v *HistogramVec) WithLabelValues(values ...string) Observer {
	if len(values) != len(v.labelNames) {
		panic("prometheus: inconsistent label cardinality")
	}
	key := strings.Join(values, "\xff")
	v.mu.Lock()
	defer v.mu.Unlock()
	if h, ok := v.metrics[key]; ok {
		return h
	}
	labels := make(map[string]string, len(values))
	for i, name := range v.labelNames {
		labels[name] = values[i]
	}
	h := newHistogram(v.opts, labels)
	v.metrics[key] = h
	return h
}

func (v *HistogramVec) metricFamilies() []*MetricFamily {
	v.mu.Lock()
	defer v.mu.Unlock()
	metrics := make([]Metric, 0, len(v.metrics))
	for _, h := range v.metrics {
		metrics = append(metrics, h.snapshot())
	}
	return []*MetricFamily{{
		Name:    v.opts.Name,
		Help:    v.opts.Help,
		Type:    HistogramValue,
		Metrics: metrics,
	}}
}

type counterMetric struct {
	mu     sync.Mutex
	value  float64
	labels map[string]string
	opts   CounterOpts
}

// NewCounter constructs a counter collector.
func NewCounter(opts CounterOpts) *counterMetric {
	return newCounter(opts, nil)
}

func newCounter(opts CounterOpts, labels map[string]string) *counterMetric {
	copied := make(map[string]string, len(labels))
	for k, v := range labels {
		copied[k] = v
	}
	return &counterMetric{opts: opts, labels: copied}
}

func (c *counterMetric) Inc() {
	c.Add(1)
}

func (c *counterMetric) Add(v float64) {
	if v < 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value += v
}

func (c *counterMetric) metricFamilies() []*MetricFamily {
	return []*MetricFamily{{
		Name: c.opts.Name,
		Help: c.opts.Help,
		Type: CounterValue,
		Metrics: []Metric{{
			Labels: copyLabels(c.labels),
			Value:  c.get(),
		}},
	}}
}

func (c *counterMetric) get() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.value
}

type CounterVec struct {
	opts       CounterOpts
	labelNames []string
	mu         sync.Mutex
	metrics    map[string]*counterMetric
}

// NewCounterVec constructs a counter vector collector.
func NewCounterVec(opts CounterOpts, labelNames []string) *CounterVec {
	return &CounterVec{
		opts:       opts,
		labelNames: append([]string(nil), labelNames...),
		metrics:    make(map[string]*counterMetric),
	}
}

// WithLabelValues retrieves the counter for the provided label values.
func (v *CounterVec) WithLabelValues(values ...string) Counter {
	if len(values) != len(v.labelNames) {
		panic("prometheus: inconsistent label cardinality")
	}
	key := strings.Join(values, "\xff")
	v.mu.Lock()
	defer v.mu.Unlock()
	if ctr, ok := v.metrics[key]; ok {
		return ctr
	}
	labels := make(map[string]string, len(values))
	for i, name := range v.labelNames {
		labels[name] = values[i]
	}
	ctr := newCounter(v.opts, labels)
	v.metrics[key] = ctr
	return ctr
}

func (v *CounterVec) metricFamilies() []*MetricFamily {
	v.mu.Lock()
	defer v.mu.Unlock()
	metrics := make([]Metric, 0, len(v.metrics))
	for _, ctr := range v.metrics {
		metrics = append(metrics, Metric{
			Labels: copyLabels(ctr.labels),
			Value:  ctr.get(),
		})
	}
	return []*MetricFamily{{
		Name:    v.opts.Name,
		Help:    v.opts.Help,
		Type:    CounterValue,
		Metrics: metrics,
	}}
}

// helper ensures maps are copied before exposure.
func copyLabels(labels map[string]string) map[string]string {
	copied := make(map[string]string, len(labels))
	for k, v := range labels {
		copied[k] = v
	}
	return copied
}

// ensure interfaces are satisfied.
var _ Collector = (*Histogram)(nil)
var _ Collector = (*HistogramVec)(nil)
var _ Collector = (*counterMetric)(nil)
var _ Collector = (*CounterVec)(nil)
var _ Observer = (*Histogram)(nil)
var _ Counter = (*counterMetric)(nil)
