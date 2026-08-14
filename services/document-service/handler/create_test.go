package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vamshireddy02/mindova/services/document-service/model"
)

func TestCreate_Success(t *testing.T) {
	svc := &stubService{
		createFn: func(ctx context.Context, doc *model.Document) error {
			doc.ID = "doc-123"
			return nil
		},
	}
	h := New(svc, &stubRetrievalService{})

	body := `{"name":"architecture.md","content":"Mindova is an AI knowledge platform.","content_type":"text/markdown"}`
	req := httptest.NewRequest(http.MethodPost, "/documents", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var got model.Document
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if got.ID != "doc-123" {
		t.Errorf("expected ID doc-123, got %s", got.ID)
	}
	if got.Name != "architecture.md" {
		t.Errorf("expected name architecture.md, got %s", got.Name)
	}
}

func TestCreate_MalformedJSON(t *testing.T) {
	svc := &stubService{
		createFn: func(ctx context.Context, doc *model.Document) error {
			t.Fatal("service should not be called for malformed JSON")
			return nil
		},
	}
	h := New(svc, &stubRetrievalService{})

	req := httptest.NewRequest(http.MethodPost, "/documents", bytes.NewReader([]byte(`{not valid json`)))
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestCreate_MissingFields(t *testing.T) {
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
				createFn: func(ctx context.Context, doc *model.Document) error {
					t.Fatal("service should not be called when fields are missing")
					return nil
				},
			}
			h := New(svc, &stubRetrievalService{})

			req := httptest.NewRequest(http.MethodPost, "/documents", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			h.Create(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400, got %d", rec.Code)
			}
		})
	}
}

func TestCreate_ServiceError(t *testing.T) {
	svc := &stubService{
		createFn: func(ctx context.Context, doc *model.Document) error {
			return errors.New("database exploded")
		},
	}
	h := New(svc, &stubRetrievalService{})

	body := `{"name":"x.md","content":"content","content_type":"text/markdown"}`
	req := httptest.NewRequest(http.MethodPost, "/documents", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
}

func TestCreate_WrongMethod(t *testing.T) {
	svc := &stubService{
		createFn: func(ctx context.Context, doc *model.Document) error {
			t.Fatal("service should not be called for wrong method")
			return nil
		},
	}
	h := New(svc, &stubRetrievalService{})

	req := httptest.NewRequest(http.MethodGet, "/documents", nil)
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", rec.Code)
	}
}

func TestCreate_UnknownField(t *testing.T) {
	svc := &stubService{
		createFn: func(ctx context.Context, doc *model.Document) error {
			t.Fatal("service should not be called for unknown fields")
			return nil
		},
	}

	h := New(svc, &stubRetrievalService{})

	body := `{
		"name": "test.md",
		"content": "hello",
		"content_type": "text/plain",
		"unknown": "should fail"
	}`

	req := httptest.NewRequest(
		http.MethodPost,
		"/documents",
		strings.NewReader(body),
	)

	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestCreate_MultipleJSONObjects(t *testing.T) {
	svc := &stubService{
		createFn: func(ctx context.Context, doc *model.Document) error {
			t.Fatal("service should not be called")
			return nil
		},
	}

	h := New(svc, &stubRetrievalService{})

	body := `{"name":"a","content":"x","content_type":"text/plain"}
{"name":"b","content":"y","content_type":"text/plain"}`

	req := httptest.NewRequest(
		http.MethodPost,
		"/documents",
		strings.NewReader(body),
	)

	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestCreate_BodyTooLarge(t *testing.T) {
	svc := &stubService{
		createFn: func(ctx context.Context, doc *model.Document) error {
			t.Fatal("service should not be called for oversized body")
			return nil
		},
	}

	h := New(svc, &stubRetrievalService{})

	largeContent := strings.Repeat("a", maxJSONBodySize+1)

	body := `{"name":"large.md","content":"` + largeContent + `","content_type":"text/plain"}`

	req := httptest.NewRequest(
		http.MethodPost,
		"/documents",
		strings.NewReader(body),
	)

	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf(
			"expected status 413, got %d, body=%s",
			rec.Code,
			rec.Body.String(),
		)
	}
}
