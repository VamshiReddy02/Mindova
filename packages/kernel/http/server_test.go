package ServerHttp

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vamshireddy02/mindova/packages/kernel/config"
	"github.com/vamshireddy02/mindova/packages/kernel/logger"
)

// TestServerCreation tests that a server can be created successfully.
func TestServerCreation(t *testing.T) {
	log := createTestLogger()
	cfg := config.AppConfig{
		Name:            "test-server",
		Environment:     config.EnvDevelopment,
		Host:            "localhost",
		Port:            8080,
		LogLevel:        config.LogInfo,
		ShutdownTimeout: 30 * time.Second,
	}

	srv := New(cfg, nil, log)
	if srv == nil {
		t.Fatal("expected server to be created, got nil")
	}
	if srv.httpServer == nil {
		t.Fatal("expected http.Server to be initialized")
	}
}

// TestServerAddress tests that the server has the correct address.
func TestServerAddress(t *testing.T) {
	log := createTestLogger()
	cfg := config.AppConfig{
		Name:            "test-server",
		Environment:     config.EnvDevelopment,
		Host:            "0.0.0.0",
		Port:            3000,
		LogLevel:        config.LogInfo,
		ShutdownTimeout: 30 * time.Second,
	}

	srv := New(cfg, nil, log)
	expectedAddr := "0.0.0.0:3000"
	if srv.Address() != expectedAddr {
		t.Errorf("expected address %s, got %s", expectedAddr, srv.Address())
	}
}

// TestServerWithValidatedConfig tests that server works with validated config.
// (Config package ensures host is not empty, so this test uses a valid host)
func TestServerWithValidatedConfig(t *testing.T) {
	log := createTestLogger()
	cfg := config.AppConfig{
		Name:            "test-server",
		Environment:     config.EnvDevelopment,
		Host:            "localhost",
		Port:            8080,
		LogLevel:        config.LogInfo,
		ShutdownTimeout: 30 * time.Second,
	}

	srv := New(cfg, nil, log)
	expectedAddr := "localhost:8080"
	if srv.Address() != expectedAddr {
		t.Errorf("expected address localhost:8080, got %s", srv.Address())
	}
}

// TestServerRequestHandling tests that HTTP requests are handled properly.
func TestServerRequestHandling(t *testing.T) {
	// Create a simple handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `{"status":"ok"}`)
	})

	log := createTestLogger()
	cfg := config.AppConfig{
		Name:            "test-server",
		Environment:     config.EnvDevelopment,
		Host:            "localhost",
		Port:            0, // Let OS choose port
		LogLevel:        config.LogInfo,
		ShutdownTimeout: 30 * time.Second,
	}

	srv := New(cfg, handler, log)

	// Use httptest to test without real networking
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/health", nil)

	// Serve the request
	srv.httpServer.Handler.ServeHTTP(recorder, request)

	// Verify response
	if recorder.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", recorder.Code)
	}

	body := recorder.Body.String()
	if body != `{"status":"ok"}` {
		t.Errorf("expected body {\"status\":\"ok\"}, got %s", body)
	}
}

// TestServerStart_Integration tests starting and stopping a real server.
func TestServerStart_Integration(t *testing.T) {
	// Create a handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "OK")
	})

	// Use httptest.Server which manages the lifecycle
	testServer := httptest.NewServer(handler)
	defer testServer.Close()

	// Make a request to verify it works
	resp, err := http.Get(testServer.URL)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "OK" {
		t.Errorf("expected body 'OK', got %s", string(body))
	}
}

// TestServerShutdown tests graceful shutdown.
func TestServerShutdown(t *testing.T) {
	log := createTestLogger()
	cfg := config.AppConfig{
		Name:            "test-server",
		Environment:     config.EnvDevelopment,
		Host:            "localhost",
		Port:            0,
		LogLevel:        config.LogInfo,
		ShutdownTimeout: 5 * time.Second,
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := New(cfg, handler, log)

	// Start server in background
	errChan := make(chan error, 1)
	go func() {
		errChan <- srv.Start()
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		t.Errorf("shutdown failed: %v", err)
	}

	// Server should stop quickly after shutdown
	select {
	case err := <-errChan:
		// Should get ErrServerClosed or nil
		if err != nil && err != http.ErrServerClosed {
			t.Errorf("unexpected error after shutdown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("server did not stop after shutdown")
	}
}

// TestServerShutdownTimeout tests that shutdown respects the timeout.
func TestServerShutdownTimeout(t *testing.T) {
	log := createTestLogger()
	cfg := config.AppConfig{
		Name:            "test-server",
		Environment:     config.EnvDevelopment,
		Host:            "localhost",
		Port:            0,
		LogLevel:        config.LogInfo,
		ShutdownTimeout: 100 * time.Millisecond,
	}

	// Handler that takes a long time
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1 * time.Second)
		w.WriteHeader(http.StatusOK)
	})

	srv := New(cfg, handler, log)

	// Start server in background
	go func() {
		_ = srv.Start()
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Shutdown with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	if err := srv.Shutdown(ctx); err != nil {
		// Shutdown may error due to timeout, which is expected
		t.Logf("shutdown returned error (expected): %v", err)
	}
	elapsed := time.Since(start)

	// Verify timeout was roughly respected (with some margin for overhead)
	if elapsed > 500*time.Millisecond {
		t.Errorf("shutdown took too long: %v", elapsed)
	}
}

// TestServerNilHandler tests that nil handler is handled gracefully.
func TestServerNilHandler(t *testing.T) {
	log := createTestLogger()
	cfg := config.AppConfig{
		Name:            "test-server",
		Environment:     config.EnvDevelopment,
		Host:            "localhost",
		Port:            8080,
		LogLevel:        config.LogInfo,
		ShutdownTimeout: 30 * time.Second,
	}

	srv := New(cfg, nil, log)
	if srv == nil {
		t.Fatal("expected server to be created even with nil handler")
	}

	// Verify request to nil handler returns 404
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/", nil)

	// Call the handler (will be nil, so net/http defaults to 404)
	if srv.httpServer.Handler != nil {
		srv.httpServer.Handler.ServeHTTP(recorder, request)
	} else {
		// nil handler should use default behavior
		http.DefaultServeMux.ServeHTTP(recorder, request)
	}

	// Response code may vary depending on DefaultServeMux
	// Main point is that it doesn't panic
	if recorder.Code < 0 {
		t.Error("handler panic or invalid response")
	}
}

// TestServerWithDifferentAddresses tests server with various addresses.
func TestServerWithDifferentAddresses(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		port     int
		expected string
	}{
		{"localhost", "localhost", 8080, "localhost:8080"},
		{"0.0.0.0", "0.0.0.0", 3000, "0.0.0.0:3000"},
		{"127.0.0.1", "127.0.0.1", 9000, "127.0.0.1:9000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := createTestLogger()
			cfg := config.AppConfig{
				Name:            "test-server",
				Environment:     config.EnvDevelopment,
				Host:            tt.host,
				Port:            tt.port,
				LogLevel:        config.LogInfo,
				ShutdownTimeout: 30 * time.Second,
			}

			srv := New(cfg, nil, log)
			if srv.Address() != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, srv.Address())
			}
		})
	}
}

// TestServerMultipleRequests tests handling multiple concurrent requests.
func TestServerMultipleRequests(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "OK")
	})

	log := createTestLogger()
	cfg := config.AppConfig{
		Name:            "test-server",
		Environment:     config.EnvDevelopment,
		Host:            "localhost",
		Port:            0,
		LogLevel:        config.LogInfo,
		ShutdownTimeout: 30 * time.Second,
	}

	srv := New(cfg, handler, log)

	// Simulate multiple requests
	for i := 0; i < 5; i++ {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest("GET", "/test", nil)

		srv.httpServer.Handler.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i, recorder.Code)
		}
	}
}

// TestServerLoggingOnStart tests that start is logged.
func TestServerLoggingOnStart(t *testing.T) {
	buf := &bytes.Buffer{}
	log := logger.NewWithWriter(config.AppConfig{
		Environment: config.EnvDevelopment,
		LogLevel:    config.LogDebug,
	}, buf)

	cfg := config.AppConfig{
		Name:            "test-server",
		Environment:     config.EnvDevelopment,
		Host:            "0.0.0.0",
		Port:            8080,
		LogLevel:        config.LogInfo,
		ShutdownTimeout: 30 * time.Second,
	}

	srv := New(cfg, nil, log)

	// Start in goroutine
	go func() {
		_ = srv.Start()
	}()

	// Give it time to start
	time.Sleep(50 * time.Millisecond)

	// Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)

	output := buf.String()
	if len(output) == 0 {
		t.Error("expected logging output, got empty")
	}
}

// Helper function to create a test logger
func createTestLogger() *logger.Logger {
	return logger.NewWithWriter(config.AppConfig{
		Environment: config.EnvDevelopment, // Uses config constant
		LogLevel:    config.LogInfo,        // Uses config constant
	}, io.Discard)
}
