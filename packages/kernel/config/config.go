package config

import "time"

type Config struct {
	App AppConfig
}

type AppConfig struct {
	Name            string
	Environment     string
	Host            string
	Port            int
	LogLevel        string
	ShutdownTimeout time.Duration
}

const (
	EnvDevelopment = "development"
	EnvStaging     = "staging"
	EnvProduction  = "production"
)

const (
	LogDebug = "debug"
	LogInfo  = "info"
	LogWarn  = "warn"
	LogError = "error"
)

var validEnvironments = []string{EnvDevelopment, EnvStaging, EnvProduction}

var validLogLevels = []string{LogDebug, LogInfo, LogWarn, LogError}

func isValidEnvironment(env string) bool {
	for _, valid := range validEnvironments {
		if env == valid {
			return true
		}
	}
	return false
}

func isValidLogLevel(level string) bool {
	for _, valid := range validLogLevels {
		if level == valid {
			return true
		}
	}
	return false
}
