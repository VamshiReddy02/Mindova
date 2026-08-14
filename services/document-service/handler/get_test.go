package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vamshireddy02/mindova/services/document-service/model"
	"github.com/vamshireddy02/mindova/services/document-service/repository"
)

func TestGetByID_Success(t *testing.T) {
	want := &model.Document{
		ID:          "doc-123",
		Name:        "architecture.md",
		Content:     "Mindova is an AI knowledge platform.",
		ContentType: "text/markdown",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	svc := &stubService{
		getByIDFn: func(ctx context.Context, id string) (*model.Document, error) {
			if id != "doc-123" {
				t.Fatalf("expected id doc-123, got %s", id)
			}
			return want, nil
		},
	}
	h := New(svc, &stubRetrievalService{}, &stubRAGService{})

	req := httptest.NewRequest(http.MethodGet, "/documents/doc-123", nil)
	rec := httptest.NewRecorder()

	h.GetByID(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var got model.Document
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if got.ID != want.ID {
		t.Errorf("expected ID %s, got %s", want.ID, got.ID)
	}
	if got.Name != want.Name {
		t.Errorf("expected name %s, got %s", want.Name, got.Name)
	}
}

func TestGetByID_NotFound(t *testing.T) {
	svc := &stubService{
		getByIDFn: func(ctx context.Context, id string) (*model.Document, error) {
			return nil, repository.ErrNotFound
		},
	}
	h := New(svc, &stubRetrievalService{}, &stubRAGService{})

	req := httptest.NewRequest(http.MethodGet, "/documents/does-not-exist", nil)
	rec := httptest.NewRecorder()

	h.GetByID(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}

	var got errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if got.Error != "document not found" {
		t.Errorf("expected error message 'document not found', got %q", got.Error)
	}
}

func TestGetByID_ServiceError(t *testing.T) {
	svc := &stubService{
		getByIDFn: func(ctx context.Context, id string) (*model.Document, error) {
			return nil, errUnimplemented // stand-in for any unexpected error
		},
	}
	h := New(svc, &stubRetrievalService{}, &stubRAGService{})

	req := httptest.NewRequest(http.MethodGet, "/documents/doc-123", nil)
	rec := httptest.NewRecorder()

	h.GetByID(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
}

func TestGetByID_MissingID(t *testing.T) {
	svc := &stubService{
		getByIDFn: func(ctx context.Context, id string) (*model.Document, error) {
			t.Fatal("service should not be called when id is missing")
			return nil, nil
		},
	}
	h := New(svc, &stubRetrievalService{}, &stubRAGService{})

	req := httptest.NewRequest(http.MethodGet, "/documents/", nil)
	rec := httptest.NewRecorder()

	h.GetByID(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestGetByID_WrongMethod(t *testing.T) {
	svc := &stubService{
		getByIDFn: func(ctx context.Context, id string) (*model.Document, error) {
			t.Fatal("service should not be called for wrong method")
			return nil, nil
		},
	}
	h := New(svc, &stubRetrievalService{}, &stubRAGService{})

	req := httptest.NewRequest(http.MethodPost, "/documents/doc-123", nil)
	rec := httptest.NewRecorder()

	h.GetByID(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", rec.Code)
	}
}
