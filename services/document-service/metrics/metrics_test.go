package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetrics_RecordsCompletedIngestion(t *testing.T) {
	m := New()

	m.RecordCompleted()
	m.RecordCompleted()
	m.RecordCompleted()

	got := m.Snapshot().IngestionsCompletedTotal
	if got != 3 {
		t.Errorf("expected ingestions_completed_total = 3, got %d", got)
	}

	// Recording completions must not touch unrelated counters.
	snap := m.Snapshot()
	if snap.IngestionsFailedTotal != 0 || snap.IngestionsRetriedTotal != 0 {
		t.Errorf("expected only completed to be incremented, got %+v", snap)
	}
}

func TestMetrics_RecordsFailedIngestion(t *testing.T) {
	m := New()

	m.RecordFailed()
	m.RecordFailed()

	got := m.Snapshot().IngestionsFailedTotal
	if got != 2 {
		t.Errorf("expected ingestions_failed_total = 2, got %d", got)
	}

	snap := m.Snapshot()
	if snap.IngestionsCompletedTotal != 0 || snap.IngestionsRetriedTotal != 0 {
		t.Errorf("expected only failed to be incremented, got %+v", snap)
	}
}

func TestMetrics_RecordsRetry(t *testing.T) {
	m := New()

	m.RecordRetried()
	m.RecordRetried()
	m.RecordRetried()
	m.RecordRetried()

	got := m.Snapshot().IngestionsRetriedTotal
	if got != 4 {
		t.Errorf("expected ingestions_retried_total = 4, got %d", got)
	}

	snap := m.Snapshot()
	if snap.IngestionsCompletedTotal != 0 || snap.IngestionsFailedTotal != 0 {
		t.Errorf("expected only retried to be incremented, got %+v", snap)
	}
}

func TestMetrics_RecordsProcessingDuration(t *testing.T) {
	m := New()

	m.RecordProcessingDuration(0.02) // falls in the smallest bucket (<=0.05)
	m.RecordProcessingDuration(0.2)  // falls in the 0.25 bucket
	m.RecordProcessingDuration(45.0) // exceeds every finite bucket -> only +Inf

	snap := m.Snapshot()
	if snap.DurationCount != 3 {
		t.Fatalf("expected 3 observations recorded, got %d", snap.DurationCount)
	}

	wantSum := 0.02 + 0.2 + 45.0
	if diff := snap.DurationSum - wantSum; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("expected duration sum %v, got %v", wantSum, snap.DurationSum)
	}

	// Verify bucket placement via the rendered text — this also doubles
	// as a first check that WriteTo produces something sane before
	// TestMetrics_Handler checks it more thoroughly.
	var buf strings.Builder
	if _, err := m.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo() returned error: %v", err)
	}
	body := buf.String()

	if !strings.Contains(body, `ingestion_processing_duration_seconds_bucket{le="0.05"} 1`) {
		t.Errorf("expected the 0.02s observation counted in the le=0.05 bucket, got:\n%s", body)
	}
	if !strings.Contains(body, `ingestion_processing_duration_seconds_bucket{le="+Inf"} 3`) {
		t.Errorf("expected all 3 observations counted in the +Inf bucket, got:\n%s", body)
	}
	if !strings.Contains(body, "ingestion_processing_duration_seconds_count 3") {
		t.Errorf("expected count 3 in rendered output, got:\n%s", body)
	}
}

func TestMetrics_Handler(t *testing.T) {
	m := New()
	m.RecordIngestionStarted()
	m.RecordCompleted()
	m.RecordFailed()
	m.RecordRetried()
	m.RecordProcessingDuration(0.3)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	m.Handler()(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/plain") {
		t.Errorf("expected Content-Type text/plain, got %q", contentType)
	}

	body := rec.Body.String()

	wantSubstrings := []string{
		"# TYPE ingestions_total counter",
		"ingestions_total 1",
		"# TYPE ingestions_completed_total counter",
		"ingestions_completed_total 1",
		"# TYPE ingestions_failed_total counter",
		"ingestions_failed_total 1",
		"# TYPE ingestions_retried_total counter",
		"ingestions_retried_total 1",
		"# TYPE ingestion_processing_duration_seconds histogram",
		"ingestion_processing_duration_seconds_sum",
		"ingestion_processing_duration_seconds_count 1",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(body, want) {
			t.Errorf("expected response body to contain %q, got:\n%s", want, body)
		}
	}
}

func TestMetrics_Handler_WrongMethod(t *testing.T) {
	m := New()

	req := httptest.NewRequest(http.MethodPost, "/metrics", nil)
	rec := httptest.NewRecorder()

	m.Handler()(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", rec.Code)
	}
}

func TestMetrics_RecordIngestionStarted(t *testing.T) {
	m := New()

	m.RecordIngestionStarted()
	m.RecordIngestionStarted()

	got := m.Snapshot().IngestionsTotal
	if got != 2 {
		t.Errorf("expected ingestions_total = 2, got %d", got)
	}
}

func TestMetrics_DefaultIsANonNilSingleton(t *testing.T) {
	if Default == nil {
		t.Fatal("expected package-level Default to be a non-nil *Metrics")
	}
}
