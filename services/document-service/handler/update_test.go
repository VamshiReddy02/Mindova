package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vamshireddy02/mindova/services/document-service/model"
	"github.com/vamshireddy02/mindova/services/document-service/repository"
)

func TestUpdate_Success(t *testing.T) {
	svc := &stubService{
		updateFn: func(ctx context.Context, doc *model.Document) error {
			if doc.ID != "doc-123" {
				t.Fatalf("expected id doc-123, got %s", doc.ID)
			}
			return nil
		},
	}
	h := New(svc, &stubRetrievalService{}, &stubRAGService{})

	body := `{"name":"renamed.md","content":"updated content","content_type":"text/markdown"}`
	req := httptest.NewRequest(http.MethodPut, "/documents/doc-123", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var got model.Document
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Name != "renamed.md" {
		t.Errorf("expected name renamed.md, got %s", got.Name)
	}
}

func TestUpdate_NotFound(t *testing.T) {
	svc := &stubService{
		updateFn: func(ctx context.Context, doc *model.Document) error {
			return repository.ErrNotFound
		},
	}
	h := New(svc, &stubRetrievalService{}, &stubRAGService{})

	body := `{"name":"x.md","content":"x","content_type":"text/plain"}`
	req := httptest.NewRequest(http.MethodPut, "/documents/does-not-exist", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}
}

func TestUpdate_MalformedJSON(t *testing.T) {
	svc := &stubService{
		updateFn: func(ctx context.Context, doc *model.Document) error {
			t.Fatal("service should not be called for malformed JSON")
			return nil
		},
	}
	h := New(svc, &stubRetrievalService{}, &stubRAGService{})

	req := httptest.NewRequest(http.MethodPut, "/documents/doc-123", strings.NewReader(`{bad json`))
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestUpdate_MissingFields(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"missing name", `{"content":"x","content_type":"text/plain"}`},
		{"missing content", `{"name":"x","content_type":"text/plain"}`},
		{"missing content_type", `{"name":"x","content":"x"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &stubService{
				updateFn: func(ctx context.Context, doc *model.Document) error {
					t.Fatal("service should not be called when fields are missing")
					return nil
				},
			}
			h := New(svc, &stubRetrievalService{}, &stubRAGService{})

			req := httptest.NewRequest(http.MethodPut, "/documents/doc-123", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			h.Update(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400, got %d", rec.Code)
			}
		})
	}
}

func TestUpdate_MissingID(t *testing.T) {
	svc := &stubService{
		updateFn: func(ctx context.Context, doc *model.Document) error {
			t.Fatal("service should not be called when id is missing")
			return nil
		},
	}
	h := New(svc, &stubRetrievalService{}, &stubRAGService{})

	body := `{"name":"x.md","content":"x","content_type":"text/plain"}`
	req := httptest.NewRequest(http.MethodPut, "/documents/", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestUpdate_ServiceError(t *testing.T) {
	svc := &stubService{
		updateFn: func(ctx context.Context, doc *model.Document) error {
			return errUnimplemented
		},
	}
	h := New(svc, &stubRetrievalService{}, &stubRAGService{})

	body := `{"name":"x.md","content":"x","content_type":"text/plain"}`
	req := httptest.NewRequest(http.MethodPut, "/documents/doc-123", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
}

func TestUpdate_WrongMethod(t *testing.T) {
	svc := &stubService{
		updateFn: func(ctx context.Context, doc *model.Document) error {
			t.Fatal("service should not be called for wrong method")
			return nil
		},
	}
	h := New(svc, &stubRetrievalService{}, &stubRAGService{})

	req := httptest.NewRequest(http.MethodGet, "/documents/doc-123", nil)
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", rec.Code)
	}
}

func TestUpdate_UnknownField(t *testing.T) {
	svc := &stubService{
		updateFn: func(ctx context.Context, doc *model.Document) error {
			t.Fatal("service should not be called for unknown fields")
			return nil
		},
	}

	h := New(svc, &stubRetrievalService{}, &stubRAGService{})

	body := `{
		"name": "test.md",
		"content": "hello",
		"content_type": "text/plain",
		"unknown": "should fail"
	}`

	req := httptest.NewRequest(
		http.MethodPut,
		"/documents/doc-123",
		strings.NewReader(body),
	)

	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestUpdate_MultipleJSONObjects(t *testing.T) {
	svc := &stubService{
		updateFn: func(ctx context.Context, doc *model.Document) error {
			t.Fatal("service should not be called")
			return nil
		},
	}

	h := New(svc, &stubRetrievalService{}, &stubRAGService{})

	body := `{"name":"a","content":"x","content_type":"text/plain"}
{"name":"b","content":"y","content_type":"text/plain"}`

	req := httptest.NewRequest(
		http.MethodPut,
		"/documents/doc-123",
		strings.NewReader(body),
	)

	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}
