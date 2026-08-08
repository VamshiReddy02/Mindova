package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func Load() (*Config, error) {
	name := strings.TrimSpace(os.Getenv("APP_NAME"))

	env := strings.TrimSpace(getEnvWithDefault("APP_ENV", EnvDevelopment))
	host := strings.TrimSpace(getEnvWithDefault("APP_HOST", "localhost"))
	logLevel := strings.TrimSpace(getEnvWithDefault("APP_LOG_LEVEL", LogInfo))

	portStr := getEnvWithDefault("APP_PORT", "8080")
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, NewConfigError(fmt.Sprintf("invalid port: %q must be a valid integer", portStr))
	}

	timeoutStr := getEnvWithDefault("APP_SHUTDOWN_TIMEOUT", "30s")
	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		return nil, NewConfigError(fmt.Sprintf("invalid shutdown timeout: %q must be a valid duration (e.g., 30s, 1m)", timeoutStr))
	}

	cfg := &Config{
		App: AppConfig{
			Name:            name,
			Environment:     env,
			Host:            host,
			Port:            port,
			LogLevel:        logLevel,
			ShutdownTimeout: timeout,
		},
	}

	if err := Validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil

}

func getEnvWithDefault(key, defaultVal string) string {
	val, ok := os.LookupEnv(key)
	if !ok {
		return defaultVal
	}
	return val
}
