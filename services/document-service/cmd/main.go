package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vamshireddy02/mindova/packages/kernel/config"
	"github.com/vamshireddy02/mindova/packages/kernel/health"
	httpkernel "github.com/vamshireddy02/mindova/packages/kernel/http"
	"github.com/vamshireddy02/mindova/packages/kernel/logger"
	"github.com/vamshireddy02/mindova/packages/kernel/middleware"

	"github.com/vamshireddy02/mindova/services/document-service/database"
	"github.com/vamshireddy02/mindova/services/document-service/handler"
	"github.com/vamshireddy02/mindova/services/document-service/repository"
	"github.com/vamshireddy02/mindova/services/document-service/service"
)

func main() {
	// 1. Load config
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	// 2. Create logger
	log := logger.New(config.AppConfig{
		Environment: cfg.App.Environment,
		LogLevel:    cfg.App.LogLevel,
	})

	// 3. Build application
	server, db, err := buildApp(cfg, log)
	if err != nil {
		log.Error("failed to build application", "error", err.Error())
		os.Exit(1)
	}
	defer db.Close()

	// 4. Start server
	go func() {
		if err := server.Start(); err != nil {
			log.Error("server failed to start", "error", err.Error())
		}
	}()

	log.Info(
		"document service started",
		"name", cfg.App.Name,
		"address", server.Address(),
	)

	// 5. Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Info("shutdown signal received")

	// 6. Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(
		context.Background(),
		cfg.App.ShutdownTimeout,
	)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", "error", err.Error())
	}

	log.Info("service stopped")
}

// buildApp creates the database, repository, service, handlers,
// router, middleware and HTTP server.
func buildApp(
	cfg *config.Config,
	log *logger.Logger,
) (*httpkernel.Server, interface {
	Close()
}, error) {

	// Connect to PostgreSQL
	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	db, err := database.New(ctx, cfg.Database)
	if err != nil {
		return nil, nil, err
	}

	log.Info(
		"connected to database",
		"host", cfg.Database.Host,
		"name", cfg.Database.Name,
	)

	// Repository
	repo := repository.New(db)

	// Service
	svc := service.New(repo)

	// Handler
	h := handler.New(svc)

	// Router
	mux := http.NewServeMux()

	// /documents
	mux.HandleFunc("/documents", documentsHandler(h))

	// /documents/{id}
	mux.HandleFunc("/documents/", documentHandler(h))

	// Health
	mux.HandleFunc("/health", health.HealthHandler)
	mux.HandleFunc("/ready", health.ReadyHandler)

	// Middleware
	finalHandler := middleware.RequestID(
		middleware.RequestLogging(log)(
			middleware.Recovery(log)(
				mux,
			),
		),
	)

	// HTTP server
	server := httpkernel.New(
		cfg.App,
		finalHandler,
		log,
	)

	return server, db, nil
}

// documentsHandler dispatches requests to /documents by HTTP method.
func documentsHandler(h *handler.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			h.Create(w, r)

		case http.MethodGet:
			h.List(w, r)

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

// documentHandler dispatches requests to /documents/{id} by HTTP method.
func documentHandler(h *handler.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			h.GetByID(w, r)

		case http.MethodPut:
			h.Update(w, r)

		case http.MethodDelete:
			h.Delete(w, r)

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}
