package middleware

import (
	"context"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRequestIDGenerates tests that middleware generates an ID when missing.
func TestRequestIDGenerates(t *testing.T) {
	// Create a simple handler that does nothing
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with request ID middleware
	middleware := RequestID(handler)

	// Create request WITHOUT X-Request-ID header
	request := httptest.NewRequest("GET", "/health", nil)
	recorder := httptest.NewRecorder()

	// Call middleware
	middleware.ServeHTTP(recorder, request)

	// Verify response status
	if recorder.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", recorder.Code)
	}

	// Verify X-Request-ID header exists in response
	requestID := recorder.Header().Get("X-Request-ID")
	if requestID == "" {
		t.Error("expected X-Request-ID header in response, got empty")
	}

	// Verify ID is not empty and is valid hex
	if len(requestID) == 0 {
		t.Error("request ID is empty")
	}

	if _, err := hex.DecodeString(requestID); err != nil {
		t.Errorf("request ID is not valid hex: %v", err)
	}

	// Verify ID length (should be 32 chars for 16 bytes)
	expectedLen := requestIDLen * 2 // hex encoding doubles the length
	if len(requestID) != expectedLen {
		t.Errorf("expected ID length %d, got %d", expectedLen, len(requestID))
	}
}

// TestRequestIDPreservesExisting tests that middleware preserves existing request IDs.
func TestRequestIDPreservesExisting(t *testing.T) {
	testID := "abc123def456"

	// Create a simple handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with request ID middleware
	middleware := RequestID(handler)

	// Create request WITH X-Request-ID header
	request := httptest.NewRequest("GET", "/health", nil)
	request.Header.Set("X-Request-ID", testID)
	recorder := httptest.NewRecorder()

	// Call middleware
	middleware.ServeHTTP(recorder, request)

	// Verify X-Request-ID header in response matches input
	responseID := recorder.Header().Get("X-Request-ID")
	if responseID != testID {
		t.Errorf("expected X-Request-ID %s, got %s", testID, responseID)
	}
}

// TestRequestIDAvailableInContext tests that ID is stored in request context.
func TestRequestIDAvailableInContext(t *testing.T) {
	testID := "test-request-id"
	var capturedID string

	// Create a handler that captures the request ID from context
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = GetRequestID(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with request ID middleware
	middleware := RequestID(handler)

	// Create request WITH X-Request-ID header
	request := httptest.NewRequest("GET", "/health", nil)
	request.Header.Set("X-Request-ID", testID)
	recorder := httptest.NewRecorder()

	// Call middleware
	middleware.ServeHTTP(recorder, request)

	// Verify handler received the correct ID from context
	if capturedID != testID {
		t.Errorf("expected context ID %s, got %s", testID, capturedID)
	}
}

// TestRequestIDGeneratedAvailableInContext tests that generated ID is available in context.
func TestRequestIDGeneratedAvailableInContext(t *testing.T) {
	var capturedID string

	// Create a handler that captures the request ID from context
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = GetRequestID(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with request ID middleware
	middleware := RequestID(handler)

	// Create request WITHOUT X-Request-ID header
	request := httptest.NewRequest("GET", "/health", nil)
	recorder := httptest.NewRecorder()

	// Call middleware
	middleware.ServeHTTP(recorder, request)

	// Verify handler received the generated ID from context
	if capturedID == "" {
		t.Error("expected generated ID in context, got empty")
	}

	// Verify it's valid hex
	if _, err := hex.DecodeString(capturedID); err != nil {
		t.Errorf("generated ID is not valid hex: %v", err)
	}
}

// TestRequestIDNextHandlerCalled tests that middleware calls the next handler.
func TestRequestIDNextHandlerCalled(t *testing.T) {
	handlerCalled := false

	// Create a handler that marks it was called
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with request ID middleware
	middleware := RequestID(handler)

	// Create request and call middleware
	request := httptest.NewRequest("GET", "/health", nil)
	recorder := httptest.NewRecorder()

	middleware.ServeHTTP(recorder, request)

	// Verify next handler was called
	if !handlerCalled {
		t.Error("middleware did not call next handler")
	}
}

// TestRequestIDMultipleCalls tests that multiple requests get different IDs.
func TestRequestIDMultipleCalls(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := RequestID(handler)

	// Make three requests without X-Request-ID
	ids := make(map[string]bool)

	for i := 0; i < 3; i++ {
		request := httptest.NewRequest("GET", "/health", nil)
		recorder := httptest.NewRecorder()

		middleware.ServeHTTP(recorder, request)

		requestID := recorder.Header().Get("X-Request-ID")
		if requestID == "" {
			t.Errorf("request %d: missing X-Request-ID", i)
		}

		// Check for duplicates
		if ids[requestID] {
			t.Errorf("duplicate request ID: %s", requestID)
		}
		ids[requestID] = true
	}

	if len(ids) != 3 {
		t.Errorf("expected 3 unique IDs, got %d", len(ids))
	}
}

// TestGetRequestIDMissing tests that GetRequestID returns empty string when missing.
func TestGetRequestIDMissing(t *testing.T) {
	ctx := context.Background()
	requestID := GetRequestID(ctx)

	if requestID != "" {
		t.Errorf("expected empty string for missing ID, got %s", requestID)
	}
}

// TestGetRequestIDFromContextDirectly tests GetRequestID retrieves value from context.
func TestGetRequestIDFromContextDirectly(t *testing.T) {
	testID := "direct-test-id"
	ctx := context.WithValue(context.Background(), requestIDKey, testID)

	requestID := GetRequestID(ctx)

	if requestID != testID {
		t.Errorf("expected %s, got %s", testID, requestID)
	}
}

// TestRequestIDWithDifferentMethods tests middleware works with various HTTP methods.
func TestRequestIDWithDifferentMethods(t *testing.T) {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := RequestID(handler)

	for _, method := range methods {
		request := httptest.NewRequest(method, "/test", nil)
		recorder := httptest.NewRecorder()

		middleware.ServeHTTP(recorder, request)

		requestID := recorder.Header().Get("X-Request-ID")
		if requestID == "" {
			t.Errorf("%s: missing X-Request-ID", method)
		}
	}
}

// TestRequestIDWithPath tests middleware works with different paths.
func TestRequestIDWithPath(t *testing.T) {
	paths := []string{"/health", "/api/users", "/api/v1/users/123"}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := RequestID(handler)

	for _, path := range paths {
		request := httptest.NewRequest("GET", path, nil)
		recorder := httptest.NewRecorder()

		middleware.ServeHTTP(recorder, request)

		requestID := recorder.Header().Get("X-Request-ID")
		if requestID == "" {
			t.Errorf("path %s: missing X-Request-ID", path)
		}
	}
}

// TestRequestIDHeaderPreservedFromUpstream tests header from upstream service is kept.
func TestRequestIDHeaderPreservedFromUpstream(t *testing.T) {
	upstreamID := "upstream-service-12345"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handler should also see the same ID
		receivedID := GetRequestID(r.Context())
		w.Header().Set("X-Received-ID", receivedID)
		w.WriteHeader(http.StatusOK)
	})

	middleware := RequestID(handler)

	// Simulate request from upstream service (with existing request ID)
	request := httptest.NewRequest("GET", "/health", nil)
	request.Header.Set("X-Request-ID", upstreamID)
	recorder := httptest.NewRecorder()

	middleware.ServeHTTP(recorder, request)

	// Verify response has same ID
	responseID := recorder.Header().Get("X-Request-ID")
	if responseID != upstreamID {
		t.Errorf("expected ID %s, got %s", upstreamID, responseID)
	}

	// Verify handler also received same ID
	receivedID := recorder.Header().Get("X-Received-ID")
	if receivedID != upstreamID {
		t.Errorf("handler: expected ID %s, got %s", upstreamID, receivedID)
	}
}

// TestRequestIDErrorResponses tests middleware works even if handler returns error.
func TestRequestIDErrorResponses(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("error"))
	})

	middleware := RequestID(handler)

	request := httptest.NewRequest("GET", "/error", nil)
	recorder := httptest.NewRecorder()

	middleware.ServeHTTP(recorder, request)

	// Verify X-Request-ID is present even for error responses
	requestID := recorder.Header().Get("X-Request-ID")
	if requestID == "" {
		t.Error("error response: missing X-Request-ID")
	}

	// Verify status is preserved
	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", recorder.Code)
	}
}
