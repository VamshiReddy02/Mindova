package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func Load() (*Config, error) {
	appCfg, err := loadAppConfig()
	if err != nil {
		return nil, err
	}

	dbCfg, err := loadDatabaseConfig()
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		App:      appCfg,
		Database: dbCfg,
	}

	if err := Validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func loadAppConfig() (AppConfig, error) {
	name := strings.TrimSpace(os.Getenv("APP_NAME"))

	env := strings.TrimSpace(getEnvWithDefault("APP_ENV", EnvDevelopment))
	host := strings.TrimSpace(getEnvWithDefault("APP_HOST", "localhost"))
	logLevel := strings.TrimSpace(getEnvWithDefault("APP_LOG_LEVEL", LogInfo))

	portStr := getEnvWithDefault("APP_PORT", "8080")
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return AppConfig{}, NewConfigError(fmt.Sprintf("invalid port: %q must be a valid integer", portStr))
	}

	timeoutStr := getEnvWithDefault("APP_SHUTDOWN_TIMEOUT", "30s")
	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		return AppConfig{}, NewConfigError(fmt.Sprintf("invalid shutdown timeout: %q must be a valid duration (e.g., 30s, 1m)", timeoutStr))
	}

	return AppConfig{
		Name:            name,
		Environment:     env,
		Host:            host,
		Port:            port,
		LogLevel:        logLevel,
		ShutdownTimeout: timeout,
	}, nil
}

func loadDatabaseConfig() (DatabaseConfig, error) {
	host := strings.TrimSpace(getEnvWithDefault("DB_HOST", "localhost"))
	user := strings.TrimSpace(getEnvWithDefault("DB_USER", "postgres"))
	password := strings.TrimSpace(getEnvWithDefault("DB_PASSWORD", "postgres"))
	name := strings.TrimSpace(getEnvWithDefault("DB_NAME", "mindova"))
	sslMode := strings.TrimSpace(getEnvWithDefault("DB_SSLMODE", SSLModeDisable))

	portStr := getEnvWithDefault("DB_PORT", "5432")
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return DatabaseConfig{}, NewConfigError(fmt.Sprintf("invalid database port: %q must be a valid integer", portStr))
	}

	maxConnsStr := getEnvWithDefault("DB_MAX_CONNS", "10")
	maxConns, err := strconv.Atoi(maxConnsStr)
	if err != nil {
		return DatabaseConfig{}, NewConfigError(fmt.Sprintf("invalid database max_conns: %q must be a valid integer", maxConnsStr))
	}

	minConnsStr := getEnvWithDefault("DB_MIN_CONNS", "2")
	minConns, err := strconv.Atoi(minConnsStr)
	if err != nil {
		return DatabaseConfig{}, NewConfigError(fmt.Sprintf("invalid database min_conns: %q must be a valid integer", minConnsStr))
	}

	lifetimeStr := getEnvWithDefault("DB_CONN_MAX_LIFETIME", "30m")
	connMaxLifetime, err := time.ParseDuration(lifetimeStr)
	if err != nil {
		return DatabaseConfig{}, NewConfigError(fmt.Sprintf("invalid database conn_max_lifetime: %q must be a valid duration (e.g., 30s, 30m)", lifetimeStr))
	}

	return DatabaseConfig{
		Host:            host,
		Port:            port,
		User:            user,
		Password:        password,
		Name:            name,
		SSLMode:         sslMode,
		MaxConns:        int32(maxConns),
		MinConns:        int32(minConns),
		ConnMaxLifetime: connMaxLifetime,
	}, nil
}

func getEnvWithDefault(key, defaultVal string) string {
	val, ok := os.LookupEnv(key)
	if !ok {
		return defaultVal
	}
	return val
}
