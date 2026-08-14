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
	"github.com/vamshireddy02/mindova/services/document-service/embedding"
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

	// 3. Connect to PostgreSQL
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	db, err := database.New(ctx, cfg.Database)
	cancel()
	if err != nil {
		log.Error("database connection failed", "error", err.Error())
		os.Exit(1)
	}
	defer db.Close()

	log.Info("connected to database", "host", cfg.Database.Host, "name", cfg.Database.Name)

	// 4. Repositories
	repo := repository.New(db)
	chunkRepo := repository.NewChunkRepo(db)

	// 5. Services
	svc := service.New(repo)

	// NOTE: must match the embedder the worker uses (cmd/run-worker) —
	// queries embedded here and chunks embedded during ingestion have to
	// come from the same model, or similarity search compares vectors
	// from two unrelated spaces and returns meaningless results.
	embeddingServiceURL := os.Getenv("EMBEDDING_SERVICE_URL")
	if embeddingServiceURL == "" {
		embeddingServiceURL = "http://localhost:8001"
	}
	embedder := embedding.NewHTTPEmbedder(embeddingServiceURL, nil)
	retrievalSvc := service.NewRetrievalService(embedder, chunkRepo)

	log.Info("using embedding service", "url", embeddingServiceURL)

	// 6. Handler
	h := handler.New(svc, retrievalSvc)

	// 7. Router
	mux := http.NewServeMux()

	// 8. Register document endpoints
	// /documents        -> POST (create), GET (list)
	// /documents/search -> POST (search) — registered explicitly so it
	//                       takes precedence over the /documents/ prefix
	//                       below; ServeMux prefers the more specific
	//                       (longer) pattern for an exact path match.
	// /documents/{id}   -> GET (get by id), PUT (update), DELETE (delete)
	mux.HandleFunc("/documents", documentsHandler(h))
	mux.HandleFunc("/documents/search", searchHandler(h))
	mux.HandleFunc("/documents/", documentHandler(h))

	// Health/readiness
	mux.HandleFunc("/health", health.HealthHandler)
	mux.HandleFunc("/ready", health.ReadyHandler)

	// 9. Middleware
	finalHandler := middleware.RequestID(
		middleware.RequestLogging(log)(
			middleware.Recovery(log)(
				mux,
			),
		),
	)

	// 10. HTTP server
	server := httpkernel.New(cfg.App, finalHandler, log)

	// 11. Start server in background
	go func() {
		if err := server.Start(); err != nil {
			log.Error("server failed to start", "error", err.Error())
		}
	}()

	log.Info("document service started",
		"name", cfg.App.Name,
		"address", server.Address(),
	)

	// 12. Wait for shutdown signal, then graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Info("shutdown signal received")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.App.ShutdownTimeout)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", "error", err.Error())
	}

	log.Info("service stopped")
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

// searchHandler dispatches requests to /documents/search by HTTP method.
func searchHandler(h *handler.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			h.Search(w, r)
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
