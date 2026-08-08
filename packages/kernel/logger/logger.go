package logger

import (
	"io"
	"log/slog"
	"os"

	"github.com/vamshireddy02/mindova/packages/kernel/config"
)

type Logger struct {
	slog *slog.Logger
}

func New(cfg config.AppConfig) *Logger {
	return NewWithWriter(cfg, os.Stderr)
}

func NewWithWriter(cfg config.AppConfig, w io.Writer) *Logger {
	var handler slog.Handler

	if isProductionEnv(cfg.Environment) {
		handler = slog.NewJSONHandler(w, &slog.HandlerOptions{
			Level: parseLogLevel(cfg.LogLevel),
		})
	} else {
		handler = slog.NewTextHandler(w, &slog.HandlerOptions{
			Level: parseLogLevel(cfg.LogLevel),
		})
	}

	return &Logger{
		slog: slog.New(handler),
	}

}

func (l *Logger) Debug(msg string, attrs ...any) {
	l.slog.Debug(msg, attrs...)
}

func (l *Logger) Info(msg string, attrs ...any) {
	l.slog.Info(msg, attrs...)
}

func (l *Logger) Warn(msg string, attrs ...any) {
	l.slog.Warn(msg, attrs...)
}

func (l *Logger) Error(msg string, attrs ...any) {
	l.slog.Error(msg, attrs...)
}

func parseLogLevel(level string) slog.Level {
	switch level {
	case config.LogDebug:
		return slog.LevelDebug
	case config.LogInfo:
		return slog.LevelInfo
	case config.LogWarn:
		return slog.LevelWarn
	case config.LogError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}

}

func isProductionEnv(env string) bool {
	return env == config.EnvProduction || env == config.EnvStaging
}
