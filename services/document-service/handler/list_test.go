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
		listFn: func(ctx context.Context, limit int) ([]*model.Document, error) {
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
		listFn: func(ctx context.Context, limit int) ([]*model.Document, error) {
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
		listFn: func(ctx context.Context, limit int) ([]*model.Document, error) {
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
		listFn: func(ctx context.Context, limit int) ([]*model.Document, error) {
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

// TestList_LimitParsing covers the query-parameter contract for ?limit=:
// default value, explicit values within range, and validation failures.
func TestList_LimitParsing(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		wantStatus    int
		wantLimit     int // only checked when wantStatus == 200
		expectSvcCall bool
	}{
		{
			name:          "no limit uses default 20",
			url:           "/documents",
			wantStatus:    http.StatusOK,
			wantLimit:     20,
			expectSvcCall: true,
		},
		{
			name:          "limit=10",
			url:           "/documents?limit=10",
			wantStatus:    http.StatusOK,
			wantLimit:     10,
			expectSvcCall: true,
		},
		{
			name:          "limit=100 at max boundary",
			url:           "/documents?limit=100",
			wantStatus:    http.StatusOK,
			wantLimit:     100,
			expectSvcCall: true,
		},
		{
			name:          "limit=101 exceeds max",
			url:           "/documents?limit=101",
			wantStatus:    http.StatusBadRequest,
			expectSvcCall: false,
		},
		{
			name:          "limit=abc is not a number",
			url:           "/documents?limit=abc",
			wantStatus:    http.StatusBadRequest,
			expectSvcCall: false,
		},
		{
			name:          "limit=0 is not positive",
			url:           "/documents?limit=0",
			wantStatus:    http.StatusBadRequest,
			expectSvcCall: false,
		},
		{
			name:          "negative limit is not positive",
			url:           "/documents?limit=-5",
			wantStatus:    http.StatusBadRequest,
			expectSvcCall: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedLimit int
			svcCalled := false

			svc := &stubService{
				listFn: func(ctx context.Context, limit int) ([]*model.Document, error) {
					svcCalled = true
					receivedLimit = limit
					return []*model.Document{}, nil
				},
			}
			h := New(svc)

			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()

			h.List(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d, body=%s", tt.wantStatus, rec.Code, rec.Body.String())
			}

			if svcCalled != tt.expectSvcCall {
				t.Fatalf("expected service called=%v, got %v", tt.expectSvcCall, svcCalled)
			}

			if tt.expectSvcCall && receivedLimit != tt.wantLimit {
				t.Errorf("expected service called with limit=%d, got %d", tt.wantLimit, receivedLimit)
			}

			if !tt.expectSvcCall {
				var got errorResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("failed to decode error response: %v", err)
				}
				if got.Error == "" {
					t.Error("expected non-empty error message")
				}
			}
		})
	}
}
