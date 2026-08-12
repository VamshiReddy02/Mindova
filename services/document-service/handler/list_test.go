package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vamshireddy02/mindova/services/document-service/model"
)

func TestList_Success(t *testing.T) {
	want := []*model.Document{
		{ID: "doc-1", Name: "a.md", Content: "aaa", ContentType: "text/markdown", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "doc-2", Name: "b.md", Content: "bbb", ContentType: "text/markdown", CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}

	svc := &stubService{
		listFn: func(ctx context.Context) ([]*model.Document, error) {
			return want, nil
		},
	}
	h := New(svc)

	req := httptest.NewRequest(http.MethodGet, "/documents", nil)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var got []*model.Document
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 documents, got %d", len(got))
	}
	if got[0].ID != "doc-1" || got[1].ID != "doc-2" {
		t.Errorf("unexpected document order/ids: %+v", got)
	}
}

func TestList_Empty(t *testing.T) {
	svc := &stubService{
		listFn: func(ctx context.Context) ([]*model.Document, error) {
			return []*model.Document{}, nil
		},
	}
	h := New(svc)

	req := httptest.NewRequest(http.MethodGet, "/documents", nil)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var got []*model.Document
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty list, got %d items", len(got))
	}
}

func TestList_ServiceError(t *testing.T) {
	svc := &stubService{
		listFn: func(ctx context.Context) ([]*model.Document, error) {
			return nil, errUnimplemented
		},
	}
	h := New(svc)

	req := httptest.NewRequest(http.MethodGet, "/documents", nil)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
}

func TestList_WrongMethod(t *testing.T) {
	svc := &stubService{
		listFn: func(ctx context.Context) ([]*model.Document, error) {
			t.Fatal("service should not be called for wrong method")
			return nil, nil
		},
	}
	h := New(svc)

	req := httptest.NewRequest(http.MethodPost, "/documents", nil)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", rec.Code)
	}
}
