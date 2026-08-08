package logger

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/vamshireddy02/mindova/packages/kernel/config"
)

// TestLoggerCreation tests that a logger can be created successfully.
func TestLoggerCreation(t *testing.T) {
	cfg := config.AppConfig{
		Environment: "development",
		LogLevel:    "info",
	}

	l := New(cfg)
	if l == nil {
		t.Fatal("expected logger to be created, got nil")
	}
	if l.slog == nil {
		t.Fatal("expected slog.Logger to be initialized")
	}
}

// TestInfoMessage tests that info messages are logged.
func TestInfoMessage(t *testing.T) {
	buf := &bytes.Buffer{}
	cfg := config.AppConfig{
		Environment: "development",
		LogLevel:    "info",
	}

	l := NewWithWriter(cfg, buf)
	l.Info("test message")

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("expected output to contain 'test message', got: %s", output)
	}
	if !strings.Contains(output, "INFO") {
		t.Errorf("expected output to contain 'INFO', got: %s", output)
	}
}

// TestDebugLevelFilter tests that debug messages are filtered when level is info.
func TestDebugLevelFilter(t *testing.T) {
	buf := &bytes.Buffer{}
	cfg := config.AppConfig{
		Environment: "development",
		LogLevel:    "info",
	}

	l := NewWithWriter(cfg, buf)
	l.Debug("debug message")
	l.Info("info message")

	output := buf.String()
	if strings.Contains(output, "debug message") {
		t.Errorf("expected debug message to be filtered out, but found it in: %s", output)
	}
	if !strings.Contains(output, "info message") {
		t.Errorf("expected info message to be present, got: %s", output)
	}
}

// TestWarnLevelFilter tests that debug and info are filtered when level is warn.
func TestWarnLevelFilter(t *testing.T) {
	buf := &bytes.Buffer{}
	cfg := config.AppConfig{
		Environment: "development",
		LogLevel:    "warn",
	}

	l := NewWithWriter(cfg, buf)
	l.Debug("debug message")
	l.Info("info message")
	l.Warn("warn message")

	output := buf.String()
	if strings.Contains(output, "debug message") {
		t.Errorf("expected debug message to be filtered out")
	}
	if strings.Contains(output, "info message") {
		t.Errorf("expected info message to be filtered out")
	}
	if !strings.Contains(output, "warn message") {
		t.Errorf("expected warn message to be present, got: %s", output)
	}
}

// TestErrorLevelFilter tests that only error messages are shown at error level.
func TestErrorLevelFilter(t *testing.T) {
	buf := &bytes.Buffer{}
	cfg := config.AppConfig{
		Environment: "development",
		LogLevel:    "error",
	}

	l := NewWithWriter(cfg, buf)
	l.Debug("debug message")
	l.Info("info message")
	l.Warn("warn message")
	l.Error("error message")

	output := buf.String()
	if strings.Contains(output, "debug message") {
		t.Errorf("expected debug message to be filtered out")
	}
	if strings.Contains(output, "info message") {
		t.Errorf("expected info message to be filtered out")
	}
	if strings.Contains(output, "warn message") {
		t.Errorf("expected warn message to be filtered out")
	}
	if !strings.Contains(output, "error message") {
		t.Errorf("expected error message to be present, got: %s", output)
	}
}

// TestStructuredAttributes tests that key-value pairs are included in the output.
func TestStructuredAttributes(t *testing.T) {
	buf := &bytes.Buffer{}
	cfg := config.AppConfig{
		Environment: "development",
		LogLevel:    "info",
	}

	l := NewWithWriter(cfg, buf)
	l.Info("user action", "user_id", 123, "action", "login")

	output := buf.String()
	if !strings.Contains(output, "user_id") {
		t.Errorf("expected output to contain 'user_id', got: %s", output)
	}
	if !strings.Contains(output, "123") {
		t.Errorf("expected output to contain '123', got: %s", output)
	}
	if !strings.Contains(output, "action") {
		t.Errorf("expected output to contain 'action', got: %s", output)
	}
	if !strings.Contains(output, "login") {
		t.Errorf("expected output to contain 'login', got: %s", output)
	}
}

// TestJSONOutputProduction tests that JSON output is used in production.
func TestJSONOutputProduction(t *testing.T) {
	buf := &bytes.Buffer{}
	cfg := config.AppConfig{
		Environment: "production",
		LogLevel:    "info",
	}

	l := NewWithWriter(cfg, buf)
	l.Info("server started", "port", 8080)

	output := buf.String()

	// Verify it's valid JSON
	var logEntry map[string]any
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		t.Fatalf("expected JSON output, got invalid JSON: %s, error: %v", output, err)
	}

	// Verify required fields
	if msg, ok := logEntry["msg"]; !ok || msg != "server started" {
		t.Errorf("expected msg field with 'server started', got: %v", logEntry)
	}
	if level, ok := logEntry["level"]; !ok || level != "INFO" {
		t.Errorf("expected level field with 'INFO', got: %v", logEntry)
	}
}

// TestJSONOutputStaging tests that JSON output is used in staging.
func TestJSONOutputStaging(t *testing.T) {
	buf := &bytes.Buffer{}
	cfg := config.AppConfig{
		Environment: "staging",
		LogLevel:    "info",
	}

	l := NewWithWriter(cfg, buf)
	l.Info("cache cleared")

	output := buf.String()

	// Verify it's valid JSON
	var logEntry map[string]any
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		t.Fatalf("expected JSON output in staging, got invalid JSON: %s", output)
	}
}

// TestTextOutputDevelopment tests that text output is used in development.
func TestTextOutputDevelopment(t *testing.T) {
	buf := &bytes.Buffer{}
	cfg := config.AppConfig{
		Environment: "development",
		LogLevel:    "info",
	}

	l := NewWithWriter(cfg, buf)
	l.Info("test message")

	output := buf.String()

	// Text format should not be JSON
	if strings.HasPrefix(strings.TrimSpace(output), "{") {
		t.Errorf("expected text output in development, got JSON: %s", output)
	}

	// Should still contain the message and level
	if !strings.Contains(output, "test message") {
		t.Errorf("expected 'test message' in output, got: %s", output)
	}
}

// TestJSONStructuredAttributes tests that structured attributes appear in JSON output.
func TestJSONStructuredAttributes(t *testing.T) {
	buf := &bytes.Buffer{}
	cfg := config.AppConfig{
		Environment: "production",
		LogLevel:    "info",
	}

	l := NewWithWriter(cfg, buf)
	l.Info("request completed", "method", "GET", "path", "/health", "status", 200)

	output := buf.String()

	// Verify it's valid JSON
	var logEntry map[string]any
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		t.Fatalf("expected valid JSON, got: %s, error: %v", output, err)
	}

	// Verify message and attributes
	if msg, ok := logEntry["msg"]; !ok || msg != "request completed" {
		t.Errorf("expected msg 'request completed', got: %v", logEntry)
	}
	if method, ok := logEntry["method"]; !ok || method != "GET" {
		t.Errorf("expected method attribute, got: %v", logEntry)
	}
	if path, ok := logEntry["path"]; !ok || path != "/health" {
		t.Errorf("expected path attribute, got: %v", logEntry)
	}
	if status, ok := logEntry["status"]; !ok || status != float64(200) {
		t.Errorf("expected status attribute, got: %v", logEntry)
	}
}

// TestMultipleLogLevels tests that all log levels work correctly.
func TestMultipleLogLevels(t *testing.T) {
	levels := []struct {
		name  string
		level string
	}{
		{"debug", "debug"},
		{"info", "info"},
		{"warn", "warn"},
		{"error", "error"},
	}

	for _, tt := range levels {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			cfg := config.AppConfig{
				Environment: "development",
				LogLevel:    tt.level,
			}

			l := NewWithWriter(cfg, buf)

			// Log at the configured level
			switch tt.level {
			case "debug":
				l.Debug("debug message")
			case "info":
				l.Info("info message")
			case "warn":
				l.Warn("warn message")
			case "error":
				l.Error("error message")
			}

			output := buf.String()
			if output == "" {
				t.Errorf("expected output for level %s, got empty", tt.level)
			}
		})
	}
}

// TestDefaultLogLevel tests that unrecognized log levels default to info.
func TestDefaultLogLevel(t *testing.T) {
	buf := &bytes.Buffer{}
	cfg := config.AppConfig{
		Environment: "development",
		LogLevel:    "invalid-level",
	}

	l := NewWithWriter(cfg, buf)
	l.Debug("debug message")
	l.Info("info message")

	output := buf.String()
	if strings.Contains(output, "debug message") {
		t.Errorf("expected debug to be filtered when level defaults to info")
	}
	if !strings.Contains(output, "info message") {
		t.Errorf("expected info message to be present when level defaults to info")
	}
}

// TestEmptyAttributes tests that logging works with no attributes.
func TestEmptyAttributes(t *testing.T) {
	buf := &bytes.Buffer{}
	cfg := config.AppConfig{
		Environment: "development",
		LogLevel:    "info",
	}

	l := NewWithWriter(cfg, buf)
	l.Info("simple message")

	output := buf.String()
	if !strings.Contains(output, "simple message") {
		t.Errorf("expected message to be logged, got: %s", output)
	}
}

// TestJSONTimeField tests that JSON output includes a time field.
func TestJSONTimeField(t *testing.T) {
	buf := &bytes.Buffer{}
	cfg := config.AppConfig{
		Environment: "production",
		LogLevel:    "info",
	}

	l := NewWithWriter(cfg, buf)
	l.Info("timed message")

	output := buf.String()

	var logEntry map[string]any
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		t.Fatalf("expected valid JSON, got: %s", output)
	}

	if _, ok := logEntry["time"]; !ok {
		t.Errorf("expected time field in JSON output, got: %v", logEntry)
	}
}
