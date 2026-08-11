package config

import (
	"fmt"
)

func Validate(cfg *Config) error {
	if err := validateApp(cfg.App); err != nil {
		return err
	}

	if err := validateDatabase(cfg.Database); err != nil {
		return err
	}

	return nil
}

func validateApp(app AppConfig) error {
	if app.Name == "" {
		return NewConfigError("missing required field: app name (set APP_NAME environment variable)")
	}

	if app.Host == "" {
		return NewConfigError("host cannot be empty")
	}

	if app.Port < 1 || app.Port > 65535 {
		return NewConfigError(fmt.Sprintf("port %d is out of valid range [1, 65535]", app.Port))
	}

	if !isValidEnvironment(app.Environment) {
		return NewConfigError(fmt.Sprintf("invalid environment: %q, must be one of [development staging production]", app.Environment))
	}

	if !isValidLogLevel(app.LogLevel) {
		return NewConfigError(fmt.Sprintf("invalid log level: %q, must be one of [debug info warn error]", app.LogLevel))
	}

	if app.ShutdownTimeout <= 0 {
		return NewConfigError(fmt.Sprintf("shutdown timeout must be positive, got %v", app.ShutdownTimeout))
	}

	return nil
}

func validateDatabase(db DatabaseConfig) error {
	if db.Host == "" {
		return NewConfigError("database host cannot be empty")
	}

	if db.Port < 1 || db.Port > 65535 {
		return NewConfigError(fmt.Sprintf("database port %d is out of valid range [1, 65535]", db.Port))
	}

	if db.User == "" {
		return NewConfigError("database user cannot be empty")
	}

	if db.Name == "" {
		return NewConfigError("database name cannot be empty")
	}

	if !isValidSSLMode(db.SSLMode) {
		return NewConfigError(fmt.Sprintf("invalid database sslmode: %q, must be one of [disable require verify-ca verify-full]", db.SSLMode))
	}

	if db.MaxConns <= 0 {
		return NewConfigError(fmt.Sprintf("database max_conns must be positive, got %d", db.MaxConns))
	}

	if db.MinConns < 0 {
		return NewConfigError(fmt.Sprintf("database min_conns cannot be negative, got %d", db.MinConns))
	}

	if db.MinConns > db.MaxConns {
		return NewConfigError(fmt.Sprintf("database min_conns (%d) cannot exceed max_conns (%d)", db.MinConns, db.MaxConns))
	}

	if db.ConnMaxLifetime <= 0 {
		return NewConfigError(fmt.Sprintf("database conn_max_lifetime must be positive, got %v", db.ConnMaxLifetime))
	}

	return nil
}

func isValidSSLMode(mode string) bool {
	switch mode {
	case SSLModeDisable, SSLModeRequire, SSLModeVerifyCA, SSLModeVerifyFull:
		return true
	default:
		return false
	}
}
