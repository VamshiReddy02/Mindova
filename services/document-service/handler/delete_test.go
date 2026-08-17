package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vamshireddy02/mindova/services/document-service/repository"
)

func TestDelete_Success(t *testing.T) {
	svc := &stubService{
		deleteFn: func(ctx context.Context, id string) error {
			if id != "doc-123" {
				t.Fatalf("expected id doc-123, got %s", id)
			}
			return nil
		},
	}
	h := New(svc, &stubRetrievalService{}, &stubRAGService{}, &stubIngestionService{}, testLogger())

	req := httptest.NewRequest(http.MethodDelete, "/documents/doc-123", nil)
	rec := httptest.NewRecorder()

	h.Delete(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Errorf("expected empty body for 204, got %q", rec.Body.String())
	}
}

func TestDelete_NotFound(t *testing.T) {
	svc := &stubService{
		deleteFn: func(ctx context.Context, id string) error {
			return repository.ErrNotFound
		},
	}
	h := New(svc, &stubRetrievalService{}, &stubRAGService{}, &stubIngestionService{}, testLogger())

	req := httptest.NewRequest(http.MethodDelete, "/documents/does-not-exist", nil)
	rec := httptest.NewRecorder()

	h.Delete(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}
}

func TestDelete_MissingID(t *testing.T) {
	svc := &stubService{
		deleteFn: func(ctx context.Context, id string) error {
			t.Fatal("service should not be called when id is missing")
			return nil
		},
	}
	h := New(svc, &stubRetrievalService{}, &stubRAGService{}, &stubIngestionService{}, testLogger())

	req := httptest.NewRequest(http.MethodDelete, "/documents/", nil)
	rec := httptest.NewRecorder()

	h.Delete(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestDelete_ServiceError(t *testing.T) {
	svc := &stubService{
		deleteFn: func(ctx context.Context, id string) error {
			return errUnimplemented
		},
	}
	h := New(svc, &stubRetrievalService{}, &stubRAGService{}, &stubIngestionService{}, testLogger())

	req := httptest.NewRequest(http.MethodDelete, "/documents/doc-123", nil)
	rec := httptest.NewRecorder()

	h.Delete(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
}

func TestDelete_WrongMethod(t *testing.T) {
	svc := &stubService{
		deleteFn: func(ctx context.Context, id string) error {
			t.Fatal("service should not be called for wrong method")
			return nil
		},
	}
	h := New(svc, &stubRetrievalService{}, &stubRAGService{}, &stubIngestionService{}, testLogger())

	req := httptest.NewRequest(http.MethodGet, "/documents/doc-123", nil)
	rec := httptest.NewRecorder()

	h.Delete(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", rec.Code)
	}
}
