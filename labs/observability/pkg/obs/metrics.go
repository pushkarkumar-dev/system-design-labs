package obs

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// MetricType identifies the kind of metric for Prometheus text format.
type MetricType string

const (
	MetricCounter   MetricType = "counter"
	MetricGauge     MetricType = "gauge"
	MetricHistogram MetricType = "histogram"
)

// Sample is a single observation: labels, value, and a Unix millisecond timestamp.
type Sample struct {
	Labels    map[string]string
	Value     float64
	Timestamp int64 // Unix ms
}

// MetricFamily is what the registry returns for one named metric.
type MetricFamily struct {
	Name    string
	Help    string
	Type    MetricType
	Samples []Sample
}

// Metric is implemented by Counter, Gauge, and Histogram.
type Metric interface {
	gather(name string) MetricFamily
}

// ---------------------------------------------------------------------------
// Counter
// ---------------------------------------------------------------------------

// Counter is a monotonically increasing float64.
// Inc() and Add() use atomic CAS — no mutex required for the fast path.
type Counter struct {
	bits   atomic.Uint64 // stores math.Float64bits(value)
	help   string
	labels map[string]string
}

// NewCounter creates a Counter with optional help text and label set.
func NewCounter(help string, labels map[string]string) *Counter {
	return &Counter{help: help, labels: cloneLabels(labels)}
}

// Inc adds 1 to the counter.
func (c *Counter) Inc() { c.Add(1) }

// Add adds n (must be >= 0) to the counter.
func (c *Counter) Add(n float64) {
	for {
		old := c.bits.Load()
		newVal := math.Float64frombits(old) + n
		if c.bits.CompareAndSwap(old, math.Float64bits(newVal)) {
			return
		}
	}
}

// Value returns the current counter value.
func (c *Counter) Value() float64 {
	return math.Float64frombits(c.bits.Load())
}

func (c *Counter) gather(name string) MetricFamily {
	return MetricFamily{
		Name: name,
		Help: c.help,
		Type: MetricCounter,
		Samples: []Sample{
			{Labels: cloneLabels(c.labels), Value: c.Value(), Timestamp: nowMs()},
		},
	}
}

// ---------------------------------------------------------------------------
// Gauge
// ---------------------------------------------------------------------------

// Gauge is a float64 that can go up or down.
type Gauge struct {
	bits   atomic.Uint64 // stores math.Float64bits(value)
	help   string
	labels map[string]string
}

// NewGauge creates a Gauge with optional help text and label set.
func NewGauge(help string, labels map[string]string) *Gauge {
	return &Gauge{help: help, labels: cloneLabels(labels)}
}

// Set sets the gauge to an exact value.
func (g *Gauge) Set(v float64) {
	g.bits.Store(math.Float64bits(v))
}

// Inc adds 1 to the gauge.
func (g *Gauge) Inc() { g.add(1) }

// Dec subtracts 1 from the gauge.
func (g *Gauge) Dec() { g.add(-1) }

func (g *Gauge) add(delta float64) {
	for {
		old := g.bits.Load()
		newVal := math.Float64frombits(old) + delta
		if g.bits.CompareAndSwap(old, math.Float64bits(newVal)) {
			return
		}
	}
}

// Value returns the current gauge value.
func (g *Gauge) Value() float64 {
	return math.Float64frombits(g.bits.Load())
}

func (g *Gauge) gather(name string) MetricFamily {
	return MetricFamily{
		Name: name,
		Help: g.help,
		Type: MetricGauge,
		Samples: []Sample{
			{Labels: cloneLabels(g.labels), Value: g.Value(), Timestamp: nowMs()},
		},
	}
}

// ---------------------------------------------------------------------------
// Histogram
// ---------------------------------------------------------------------------

// DefaultBuckets are the default histogram upper bounds,
// matching the Prometheus Go client defaults.
var DefaultBuckets = []float64{
	0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5,
	math.Inf(1), // +Inf bucket always present
}

// Histogram tracks value distributions in configurable buckets.
type Histogram struct {
	mu      sync.Mutex
	buckets []float64 // upper bounds (sorted), last must be +Inf
	counts  []uint64  // cumulative count per bucket
	sum     float64
	total   uint64
	help    string
	labels  map[string]string
}

// NewHistogram creates a Histogram. If buckets is nil, DefaultBuckets is used.
// The +Inf bucket is added automatically if not already present.
func NewHistogram(help string, labels map[string]string, buckets []float64) *Histogram {
	if len(buckets) == 0 {
		buckets = DefaultBuckets
	}
	b := make([]float64, len(buckets))
	copy(b, buckets)
	sort.Float64s(b)
	if !math.IsInf(b[len(b)-1], 1) {
		b = append(b, math.Inf(1))
	}
	return &Histogram{
		buckets: b,
		counts:  make([]uint64, len(b)),
		help:    help,
		labels:  cloneLabels(labels),
	}
}

// Observe records one observation. It increments all buckets whose upper
// bound is >= v (Prometheus cumulative convention).
func (h *Histogram) Observe(v float64) {
	// Binary search for first bucket index where buckets[i] >= v.
	idx := sort.SearchFloat64s(h.buckets, v)
	if idx == len(h.buckets) {
		idx = len(h.buckets) - 1
	}
	h.mu.Lock()
	for i := idx; i < len(h.counts); i++ {
		h.counts[i]++
	}
	h.sum += v
	h.total++
	h.mu.Unlock()
}

// BucketCount returns the cumulative count for the bucket at index i.
func (h *Histogram) BucketCount(i int) uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	if i < 0 || i >= len(h.counts) {
		return 0
	}
	return h.counts[i]
}

// Buckets returns the upper bounds slice (read-only view).
func (h *Histogram) Buckets() []float64 { return h.buckets }

func (h *Histogram) gather(name string) MetricFamily {
	h.mu.Lock()
	counts := make([]uint64, len(h.counts))
	copy(counts, h.counts)
	sum := h.sum
	total := h.total
	h.mu.Unlock()

	ts := nowMs()
	samples := make([]Sample, 0, len(h.buckets)+2)

	for i, ub := range h.buckets {
		leStr := "+Inf"
		if !math.IsInf(ub, 1) {
			leStr = fmt.Sprintf("%g", ub)
		}
		lbls := cloneLabels(h.labels)
		lbls["le"] = leStr
		samples = append(samples, Sample{Labels: lbls, Value: float64(counts[i]), Timestamp: ts})
	}
	// _sum
	sumLbls := cloneLabels(h.labels)
	sumLbls["__type__"] = "sum"
	samples = append(samples, Sample{Labels: sumLbls, Value: sum, Timestamp: ts})
	// _count
	countLbls := cloneLabels(h.labels)
	countLbls["__type__"] = "count"
	samples = append(samples, Sample{Labels: countLbls, Value: float64(total), Timestamp: ts})

	return MetricFamily{Name: name, Help: h.help, Type: MetricHistogram, Samples: samples}
}

// ---------------------------------------------------------------------------
// MetricsRegistry
// ---------------------------------------------------------------------------

// MetricsRegistry is a global store of named metrics.
type MetricsRegistry struct {
	mu      sync.RWMutex
	metrics map[string]Metric
}

// NewMetricsRegistry returns an empty registry.
func NewMetricsRegistry() *MetricsRegistry {
	return &MetricsRegistry{metrics: make(map[string]Metric)}
}

// Register adds a metric under name. Panics if name is already registered.
func (r *MetricsRegistry) Register(name string, m Metric) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.metrics[name]; exists {
		panic("obs: metric already registered: " + name)
	}
	r.metrics[name] = m
}

// Gather collects a MetricFamily from every registered metric.
// Output is sorted by metric name for deterministic Prometheus scrapes.
func (r *MetricsRegistry) Gather() []MetricFamily {
	r.mu.RLock()
	names := make([]string, 0, len(r.metrics))
	for name := range r.metrics {
		names = append(names, name)
	}
	r.mu.RUnlock()

	sort.Strings(names)
	families := make([]MetricFamily, 0, len(names))
	for _, name := range names {
		r.mu.RLock()
		m := r.metrics[name]
		r.mu.RUnlock()
		families = append(families, m.gather(name))
	}
	return families
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func nowMs() int64 { return time.Now().UnixMilli() }

// BenchMetricName returns a unique metric name for use in benchmarks.
// Exported so bench_test.go (in a different package) can use it without fmt.
func BenchMetricName(i int) string {
	const digits = "0123456789"
	prefix := "bench_metric_"
	// Simple int-to-string without importing strconv or fmt in this file.
	if i == 0 {
		return prefix + "0"
	}
	buf := make([]byte, 0, 12)
	for n := i; n > 0; n /= 10 {
		buf = append([]byte{digits[n%10]}, buf...)
	}
	return prefix + string(buf)
}

func cloneLabels(labels map[string]string) map[string]string {
	out := make(map[string]string, len(labels))
	for k, v := range labels {
		out[k] = v
	}
	return out
}
