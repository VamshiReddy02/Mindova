package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vamshireddy02/mindova/services/document-service/model"
	"github.com/vamshireddy02/mindova/services/document-service/service"
)

func TestSearch_Success(t *testing.T) {
	want := []*model.DocumentChunk{
		{ID: "chunk-1", Content: "how Mindova chunks documents"},
		{ID: "chunk-2", Content: "the ingestion pipeline overview"},
	}

	var gotQuery string
	var gotLimit int

	retrieval := &stubRetrievalService{
		searchFn: func(ctx context.Context, query string, limit int) ([]*model.DocumentChunk, error) {
			gotQuery = query
			gotLimit = limit
			return want, nil
		},
	}
	h := New(&stubService{}, retrieval, &stubRAGService{})

	body := `{"query":"How does Mindova process documents?","limit":5}`
	req := httptest.NewRequest(http.MethodPost, "/documents/search", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Search(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if gotQuery != "How does Mindova process documents?" {
		t.Errorf("expected query passed through, got %q", gotQuery)
	}
	if gotLimit != 5 {
		t.Errorf("expected limit 5 passed through, got %d", gotLimit)
	}

	var got []*model.DocumentChunk
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(got))
	}
	if got[0].ID != "chunk-1" || got[1].ID != "chunk-2" {
		t.Errorf("unexpected chunks in response: %+v", got)
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	retrieval := &stubRetrievalService{
		searchFn: func(ctx context.Context, query string, limit int) ([]*model.DocumentChunk, error) {
			return nil, service.ErrEmptyQuery
		},
	}
	h := New(&stubService{}, retrieval, &stubRAGService{})

	body := `{"query":"","limit":5}`
	req := httptest.NewRequest(http.MethodPost, "/documents/search", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Search(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestSearch_InvalidLimit(t *testing.T) {
	retrieval := &stubRetrievalService{
		searchFn: func(ctx context.Context, query string, limit int) ([]*model.DocumentChunk, error) {
			return nil, service.ErrInvalidLimit
		},
	}
	h := New(&stubService{}, retrieval, &stubRAGService{})

	body := `{"query":"a valid query","limit":0}`
	req := httptest.NewRequest(http.MethodPost, "/documents/search", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Search(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestSearch_ServiceError(t *testing.T) {
	retrieval := &stubRetrievalService{
		searchFn: func(ctx context.Context, query string, limit int) ([]*model.DocumentChunk, error) {
			return nil, errors.New("embedding provider unavailable")
		},
	}
	h := New(&stubService{}, retrieval, &stubRAGService{})

	body := `{"query":"a valid query","limit":5}`
	req := httptest.NewRequest(http.MethodPost, "/documents/search", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Search(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
}

func TestSearch_MalformedJSON(t *testing.T) {
	retrieval := &stubRetrievalService{
		searchFn: func(ctx context.Context, query string, limit int) ([]*model.DocumentChunk, error) {
			t.Fatal("service should not be called for malformed JSON")
			return nil, nil
		},
	}
	h := New(&stubService{}, retrieval, &stubRAGService{})

	req := httptest.NewRequest(http.MethodPost, "/documents/search", strings.NewReader(`{bad json`))
	rec := httptest.NewRecorder()

	h.Search(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestSearch_UnknownField(t *testing.T) {
	retrieval := &stubRetrievalService{
		searchFn: func(ctx context.Context, query string, limit int) ([]*model.DocumentChunk, error) {
			t.Fatal("service should not be called for unknown fields")
			return nil, nil
		},
	}
	h := New(&stubService{}, retrieval, &stubRAGService{})

	body := `{"query":"a query","limit":5,"unknown":"field"}`
	req := httptest.NewRequest(http.MethodPost, "/documents/search", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Search(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestSearch_MultipleJSONObjects(t *testing.T) {
	retrieval := &stubRetrievalService{
		searchFn: func(ctx context.Context, query string, limit int) ([]*model.DocumentChunk, error) {
			t.Fatal("service should not be called")
			return nil, nil
		},
	}
	h := New(&stubService{}, retrieval, &stubRAGService{})

	body := `{"query":"a","limit":5}
{"query":"b","limit":5}`
	req := httptest.NewRequest(http.MethodPost, "/documents/search", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Search(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestSearch_WrongMethod(t *testing.T) {
	retrieval := &stubRetrievalService{
		searchFn: func(ctx context.Context, query string, limit int) ([]*model.DocumentChunk, error) {
			t.Fatal("service should not be called for wrong method")
			return nil, nil
		},
	}
	h := New(&stubService{}, retrieval, &stubRAGService{})

	req := httptest.NewRequest(http.MethodGet, "/documents/search", nil)
	rec := httptest.NewRecorder()

	h.Search(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", rec.Code)
	}
}

func TestSearch_EmptyResults(t *testing.T) {
	retrieval := &stubRetrievalService{
		searchFn: func(ctx context.Context, query string, limit int) ([]*model.DocumentChunk, error) {
			return []*model.DocumentChunk{}, nil
		},
	}
	h := New(&stubService{}, retrieval, &stubRAGService{})

	body := `{"query":"nothing matches this","limit":5}`
	req := httptest.NewRequest(http.MethodPost, "/documents/search", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Search(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for a query with no matches, got %d", rec.Code)
	}

	var got []*model.DocumentChunk
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty results array, got %d chunks", len(got))
	}
}
