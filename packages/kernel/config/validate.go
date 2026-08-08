package config

import (
	"fmt"
)

func Validate(cfg *Config) error {
	app := cfg.App

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
