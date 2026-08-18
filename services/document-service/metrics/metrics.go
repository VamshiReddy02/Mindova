// Package metrics provides ingestion pipeline observability, exposed in
// Prometheus text exposition format at GET /metrics.
//
// This is a small, self-contained implementation rather than a
// dependency on the official prometheus/client_golang library — the
// output format is a standard, well-documented text protocol any
// Prometheus server (or `curl | promtool check metrics`) already knows
// how to parse, so hand-rolling it avoids pulling in a new third-party
// dependency for four counters and one histogram.
package metrics

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
)

// defaultBuckets are the histogram bucket upper bounds (in seconds) for
// ingestion_processing_duration_seconds. Chosen to span "fast" (a small
// text chunk, mock embedder) through "slow" (a real embedding provider
// or LLM call over the network) without needing per-deployment tuning
// for an MVP.
var defaultBuckets = []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60}

// Metrics tracks the ingestion pipeline's core counters and a processing
// duration histogram. Safe for concurrent use — every field is either an
// atomic or protected by durationMu — since the worker (and potentially
// multiple worker goroutines/replicas) record against the same instance
// concurrently with GET /metrics reading it.
type Metrics struct {
	ingestionsTotal          atomic.Int64
	ingestionsCompletedTotal atomic.Int64
	ingestionsFailedTotal    atomic.Int64
	ingestionsRetriedTotal   atomic.Int64

	durationMu      sync.Mutex
	durationBuckets []float64
	// durationCounts holds RAW (non-cumulative) counts: durationCounts[i]
	// is how many observations fell in (durationBuckets[i-1],
	// durationBuckets[i]]. WriteTo computes the cumulative "le" values
	// Prometheus's histogram format requires at render time — this is
	// the same non-cumulative-storage / cumulative-on-read approach
	// prometheus/client_golang itself uses internally.
	durationCounts []uint64
	durationSum    float64
	durationCount  uint64
}

// New creates a Metrics with a fresh set of counters, all starting at
// zero, and the default duration histogram buckets.
func New() *Metrics {
	return &Metrics{
		durationBuckets: append([]float64(nil), defaultBuckets...),
		durationCounts:  make([]uint64, len(defaultBuckets)),
	}
}

// Default is the process-wide Metrics instance the worker records
// against, and what GET /metrics serves by default.
//
// Kept as a package-level singleton rather than threaded through
// Worker's constructor deliberately: metrics are cross-cutting
// instrumentation, not a core dependency of ingestion logic, and adding
// it as a New() parameter would mean updating every existing call site
// across the worker, service, and handler test suites for something
// that's fundamentally observability, not behavior. Tests that want
// isolated metrics (not sharing state with other tests or with Default)
// construct their own instance via New() instead.
var Default = New()

// RecordIngestionStarted increments ingestions_total — called once per
// ingestion claimed for processing (each attempt, including retries,
// counts as a new "started").
func (m *Metrics) RecordIngestionStarted() {
	m.ingestionsTotal.Add(1)
}

// RecordCompleted increments ingestions_completed_total.
func (m *Metrics) RecordCompleted() {
	m.ingestionsCompletedTotal.Add(1)
}

// RecordFailed increments ingestions_failed_total — call only when an
// ingestion is PERMANENTLY failed (attempts exhausted), not on every
// failed attempt; use RecordRetried for the latter.
func (m *Metrics) RecordFailed() {
	m.ingestionsFailedTotal.Add(1)
}

// RecordRetried increments ingestions_retried_total — call when a failed
// attempt is requeued to pending for another try (attempts still below
// the configured maximum).
func (m *Metrics) RecordRetried() {
	m.ingestionsRetriedTotal.Add(1)
}

// RecordProcessingDuration records how long a single ingestion attempt
// took, in seconds, into the ingestion_processing_duration_seconds
// histogram. Called once per attempt regardless of whether it succeeded,
// retried, or permanently failed — duration is useful signal in all
// three cases.
func (m *Metrics) RecordProcessingDuration(seconds float64) {
	m.durationMu.Lock()
	defer m.durationMu.Unlock()

	m.durationSum += seconds
	m.durationCount++

	for i, b := range m.durationBuckets {
		if seconds <= b {
			m.durationCounts[i]++
			return
		}
	}
	// Exceeds every finite bucket: only the +Inf bucket applies, which
	// WriteTo derives from durationCount directly rather than a stored
	// bucket — every observation is <= +Inf by definition.
}

// Snapshot is a point-in-time read of every metric's current value,
// useful for tests asserting on values directly rather than parsing the
// rendered Prometheus text.
type Snapshot struct {
	IngestionsTotal          int64
	IngestionsCompletedTotal int64
	IngestionsFailedTotal    int64
	IngestionsRetriedTotal   int64
	DurationSum              float64
	DurationCount            uint64
}

func (m *Metrics) Snapshot() Snapshot {
	m.durationMu.Lock()
	defer m.durationMu.Unlock()

	return Snapshot{
		IngestionsTotal:          m.ingestionsTotal.Load(),
		IngestionsCompletedTotal: m.ingestionsCompletedTotal.Load(),
		IngestionsFailedTotal:    m.ingestionsFailedTotal.Load(),
		IngestionsRetriedTotal:   m.ingestionsRetriedTotal.Load(),
		DurationSum:              m.durationSum,
		DurationCount:            m.durationCount,
	}
}

// WriteTo renders every metric in Prometheus text exposition format
// (the widely-supported plain-text protocol; version=0.0.4 in the
// Content-Type, per convention). Implements io.WriterTo.
func (m *Metrics) WriteTo(w io.Writer) (int64, error) {
	var written int64

	write := func(format string, args ...any) error {
		n, err := fmt.Fprintf(w, format, args...)
		written += int64(n)
		return err
	}

	counters := []struct {
		name string
		help string
		val  int64
	}{
		{"ingestions_total", "Total number of ingestion attempts claimed for processing.", m.ingestionsTotal.Load()},
		{"ingestions_completed_total", "Total number of ingestion jobs that completed successfully.", m.ingestionsCompletedTotal.Load()},
		{"ingestions_failed_total", "Total number of ingestion jobs permanently marked failed after exhausting retries.", m.ingestionsFailedTotal.Load()},
		{"ingestions_retried_total", "Total number of ingestion attempts that failed but were requeued for another try.", m.ingestionsRetriedTotal.Load()},
	}

	for _, c := range counters {
		if err := write("# HELP %s %s\n", c.name, c.help); err != nil {
			return written, err
		}
		if err := write("# TYPE %s counter\n", c.name); err != nil {
			return written, err
		}
		if err := write("%s %d\n", c.name, c.val); err != nil {
			return written, err
		}
	}

	m.durationMu.Lock()
	buckets := append([]float64(nil), m.durationBuckets...)
	counts := append([]uint64(nil), m.durationCounts...)
	sum := m.durationSum
	count := m.durationCount
	m.durationMu.Unlock()

	if err := write("# HELP ingestion_processing_duration_seconds Time spent processing a single ingestion attempt, in seconds.\n"); err != nil {
		return written, err
	}
	if err := write("# TYPE ingestion_processing_duration_seconds histogram\n"); err != nil {
		return written, err
	}

	var cumulative uint64
	for i, b := range buckets {
		cumulative += counts[i]
		if err := write("ingestion_processing_duration_seconds_bucket{le=\"%s\"} %d\n", formatFloat(b), cumulative); err != nil {
			return written, err
		}
	}
	if err := write("ingestion_processing_duration_seconds_bucket{le=\"+Inf\"} %d\n", count); err != nil {
		return written, err
	}
	if err := write("ingestion_processing_duration_seconds_sum %s\n", formatFloat(sum)); err != nil {
		return written, err
	}
	if err := write("ingestion_processing_duration_seconds_count %d\n", count); err != nil {
		return written, err
	}

	return written, nil
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// Handler returns an http.HandlerFunc serving m in Prometheus text
// exposition format. Intended to be registered at GET /metrics.
func (m *Metrics) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = m.WriteTo(w)
	}
}
