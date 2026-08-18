package config

import (
	"os"
	"testing"
	"time"
)

// TestLoadValidConfig tests that valid configuration loads successfully.
func TestLoadValidConfig(t *testing.T) {
	// Set up environment variables
	t.Setenv("APP_NAME", "test-service")
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_HOST", "0.0.0.0")
	t.Setenv("APP_PORT", "3000")
	t.Setenv("APP_LOG_LEVEL", "warn")
	t.Setenv("APP_SHUTDOWN_TIMEOUT", "60s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.App.Name != "test-service" {
		t.Errorf("expected name test-service, got %s", cfg.App.Name)
	}
	if cfg.App.Environment != "production" {
		t.Errorf("expected environment production, got %s", cfg.App.Environment)
	}
	if cfg.App.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", cfg.App.Host)
	}
	if cfg.App.Port != 3000 {
		t.Errorf("expected port 3000, got %d", cfg.App.Port)
	}
	if cfg.App.LogLevel != "warn" {
		t.Errorf("expected log level warn, got %s", cfg.App.LogLevel)
	}
	if cfg.App.ShutdownTimeout != 60*time.Second {
		t.Errorf("expected timeout 60s, got %v", cfg.App.ShutdownTimeout)
	}
}

// TestLoadAppliesDefaults tests that sensible defaults are applied.
func TestLoadAppliesDefaults(t *testing.T) {
	t.Setenv("APP_NAME", "my-service")
	// Unset optional variables to use defaults
	os.Unsetenv("APP_ENV")
	os.Unsetenv("APP_HOST")
	os.Unsetenv("APP_PORT")
	os.Unsetenv("APP_LOG_LEVEL")
	os.Unsetenv("APP_SHUTDOWN_TIMEOUT")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.App.Environment != EnvDevelopment {
		t.Errorf("expected default environment %s, got %s", EnvDevelopment, cfg.App.Environment)
	}
	if cfg.App.Host != "0.0.0.0" {
		t.Errorf("expected default host 0.0.0.0, got %s", cfg.App.Host)
	}
	if cfg.App.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.App.Port)
	}
	if cfg.App.LogLevel != LogInfo {
		t.Errorf("expected default log level %s, got %s", LogInfo, cfg.App.LogLevel)
	}
	if cfg.App.ShutdownTimeout != 30*time.Second {
		t.Errorf("expected default timeout 30s, got %v", cfg.App.ShutdownTimeout)
	}
}

// TestLoadMissingRequiredValue tests that missing required values return errors.
func TestLoadMissingRequiredValue(t *testing.T) {
	os.Unsetenv("APP_NAME")

	cfg, err := Load()
	if err == nil {
		t.Fatal("expected error for missing APP_NAME, got none")
	}
	if cfg != nil {
		t.Errorf("expected nil config, got %+v", cfg)
	}
}

// TestLoadInvalidPort tests various invalid port scenarios.
func TestLoadInvalidPort(t *testing.T) {
	tests := []struct {
		name    string
		portVal string
		wantErr bool
	}{
		{"not an integer", "abc", true},
		{"negative port", "-1", true},
		{"zero port", "0", true},
		{"port too high", "70000", true},
		{"valid minimum", "1", false},
		{"valid maximum", "65535", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("APP_NAME", "test")
			t.Setenv("APP_PORT", tt.portVal)

			cfg, err := Load()
			if tt.wantErr && err == nil {
				t.Errorf("expected error for port %s", tt.portVal)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
			if !tt.wantErr && cfg != nil {
				if tt.portVal == "1" && cfg.App.Port != 1 {
					t.Errorf("expected port 1, got %d", cfg.App.Port)
				}
				if tt.portVal == "65535" && cfg.App.Port != 65535 {
					t.Errorf("expected port 65535, got %d", cfg.App.Port)
				}
			}
		})
	}
}

// TestLoadInvalidEnvironment tests invalid environment values.
func TestLoadInvalidEnvironment(t *testing.T) {
	t.Setenv("APP_NAME", "test")
	t.Setenv("APP_ENV", "invalid-env")

	cfg, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid environment")
	}
	if cfg != nil {
		t.Errorf("expected nil config, got %+v", cfg)
	}
}

// TestLoadInvalidLogLevel tests invalid log level values.
func TestLoadInvalidLogLevel(t *testing.T) {
	t.Setenv("APP_NAME", "test")
	t.Setenv("APP_LOG_LEVEL", "trace")

	cfg, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid log level")
	}
	if cfg != nil {
		t.Errorf("expected nil config, got %+v", cfg)
	}
}

// TestLoadInvalidTimeout tests invalid timeout values.
func TestLoadInvalidTimeout(t *testing.T) {
	tests := []struct {
		name       string
		timeoutVal string
		wantErr    bool
	}{
		{"not a duration", "not-a-duration", true},
		{"negative duration", "-5s", true},
		{"zero duration", "0s", true},
		{"valid duration", "45s", false},
		{"valid duration with minutes", "2m30s", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("APP_NAME", "test")
			t.Setenv("APP_SHUTDOWN_TIMEOUT", tt.timeoutVal)

			cfg, err := Load()
			if tt.wantErr && err == nil {
				t.Errorf("expected error for timeout %s", tt.timeoutVal)
			}
			if !tt.wantErr && cfg != nil {
				if cfg.App.ShutdownTimeout <= 0 {
					t.Errorf("expected positive timeout, got %v", cfg.App.ShutdownTimeout)
				}
			}
		})
	}
}

// TestLoadAllEnvironments tests all valid environment values.
func TestLoadAllEnvironments(t *testing.T) {
	validEnvs := []string{EnvDevelopment, EnvStaging, EnvProduction}
	for _, env := range validEnvs {
		t.Run(env, func(t *testing.T) {
			t.Setenv("APP_NAME", "test")
			t.Setenv("APP_ENV", env)

			cfg, err := Load()
			if err != nil {
				t.Errorf("expected no error for environment %s, got %v", env, err)
			}
			if cfg.App.Environment != env {
				t.Errorf("expected environment %s, got %s", env, cfg.App.Environment)
			}
		})
	}
}

// TestLoadAllLogLevels tests all valid log levels.
func TestLoadAllLogLevels(t *testing.T) {
	validLevels := []string{LogDebug, LogInfo, LogWarn, LogError}
	for _, level := range validLevels {
		t.Run(level, func(t *testing.T) {
			t.Setenv("APP_NAME", "test")
			t.Setenv("APP_LOG_LEVEL", level)

			cfg, err := Load()
			if err != nil {
				t.Errorf("expected no error for log level %s, got %v", level, err)
			}
			if cfg.App.LogLevel != level {
				t.Errorf("expected log level %s, got %s", level, cfg.App.LogLevel)
			}
		})
	}
}

// TestValidateEmptyHost tests that validation catches empty host.
func TestValidateEmptyHost(t *testing.T) {
	cfg := &Config{
		App: AppConfig{
			Name:            "test",
			Environment:     EnvDevelopment,
			Host:            "",
			Port:            8080,
			LogLevel:        LogInfo,
			ShutdownTimeout: 30 * time.Second,
		},
	}

	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for empty host")
	}
}

// TestValidateEmptyName tests that validation catches empty name.
func TestValidateEmptyName(t *testing.T) {
	cfg := &Config{
		App: AppConfig{
			Name:            "",
			Environment:     EnvDevelopment,
			Host:            "localhost",
			Port:            8080,
			LogLevel:        LogInfo,
			ShutdownTimeout: 30 * time.Second,
		},
	}

	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

// TestLoadWhitespaceHandling tests that whitespace is trimmed.
func TestLoadWhitespaceHandling(t *testing.T) {
	t.Setenv("APP_NAME", "  test-service  ")
	t.Setenv("APP_ENV", "  production  ")
	t.Setenv("APP_HOST", "  0.0.0.0  ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.App.Name != "test-service" {
		t.Errorf("expected trimmed name 'test-service', got '%s'", cfg.App.Name)
	}
	if cfg.App.Environment != "production" {
		t.Errorf("expected trimmed environment 'production', got '%s'", cfg.App.Environment)
	}
	if cfg.App.Host != "0.0.0.0" {
		t.Errorf("expected trimmed host '0.0.0.0', got '%s'", cfg.App.Host)
	}
}
