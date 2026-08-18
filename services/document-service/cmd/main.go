package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strconv"
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
	"github.com/vamshireddy02/mindova/services/document-service/llm"
	"github.com/vamshireddy02/mindova/services/document-service/metrics"
	"github.com/vamshireddy02/mindova/services/document-service/repository"
	"github.com/vamshireddy02/mindova/services/document-service/service"
	"github.com/vamshireddy02/mindova/services/document-service/worker"
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
	ingestionRepo := repository.NewIngestionRepo(db)

	// 5. Services
	svc := service.New(repo)
	ingestionSvc := service.NewIngestionService(ingestionRepo)

	// NOTE: must match the embedder the worker uses — queries embedded
	// here and chunks embedded during ingestion have to come from the
	// same model, or similarity search compares vectors from two
	// unrelated spaces and returns meaningless results.
	embeddingServiceURL := os.Getenv("EMBEDDING_SERVICE_URL")
	if embeddingServiceURL == "" {
		embeddingServiceURL = "http://localhost:8001"
	}
	embedder := embedding.NewHTTPEmbedder(embeddingServiceURL, nil)
	retrievalSvc := service.NewRetrievalService(embedder, chunkRepo)

	log.Info("using embedding service", "url", embeddingServiceURL)

	llmServiceURL := os.Getenv("LLM_SERVICE_URL")
	if llmServiceURL == "" {
		llmServiceURL = "http://localhost:8001"
	}
	llmClient := llm.NewHTTPClient(llmServiceURL, nil)
	ragSvc := service.NewRAGService(retrievalSvc, llmClient)

	log.Info("using llm service", "url", llmServiceURL)

	// 5b. Background ingestion worker.
	//
	// This is what makes ingestion actually automatic: POST /documents
	// enqueues a pending ingestion (see handler.Create), and this
	// long-running loop — started once, right here, alongside the HTTP
	// server — continuously picks up whatever's pending. No one has to
	// manually INSERT a row or run a separate worker binary by hand
	// anymore; that's still available at cmd/run-worker for one-off
	// debugging, but the running document-service process is now
	// self-sufficient.
	pollInterval := 5 * time.Second
	if raw := os.Getenv("WORKER_INTERVAL"); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil {
			pollInterval = parsed
		} else {
			log.Error("invalid WORKER_INTERVAL, using default", "value", raw, "default", pollInterval.String())
		}
	}

	maxAttempts := 3
	if raw := os.Getenv("MAX_ATTEMPTS"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			maxAttempts = parsed
		} else {
			log.Error("invalid MAX_ATTEMPTS, using default", "value", raw, "default", maxAttempts)
		}
	}

	processor := worker.NewTextProcessor(0)
	w := worker.New(ingestionSvc, svc, processor, embedder, chunkRepo, 0, maxAttempts)

	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	go func() {
		err := w.RunLoop(workerCtx, pollInterval, func(err error) {
			log.Error("worker pass failed", "error", err.Error())
		})
		if err != nil && err != context.Canceled {
			log.Error("worker loop exited unexpectedly", "error", err.Error())
		}
	}()

	log.Info("ingestion worker started",
		"poll_interval", pollInterval.String(),
		"max_attempts", maxAttempts,
	)

	// 6. Handler
	h := handler.New(svc, retrievalSvc, ragSvc, ingestionSvc, log)

	// 7. Router
	mux := http.NewServeMux()

	// 8. Register document endpoints
	// /documents        -> POST (create), GET (list)
	// /documents/search -> POST (search) — registered explicitly so it
	//                       takes precedence over the /documents/ prefix
	//                       below; ServeMux prefers the more specific
	//                       (longer) pattern for an exact path match.
	// /documents/ask    -> POST (RAG question answering) — same reasoning.
	// /documents/{id}   -> GET (get by id), PUT (update), DELETE (delete)
	mux.HandleFunc("/documents", documentsHandler(h))
	mux.HandleFunc("/documents/search", searchHandler(h))
	mux.HandleFunc("/documents/ask", askHandler(h))
	mux.HandleFunc("/documents/", documentHandler(h))

	// Health/readiness
	mux.HandleFunc("/health", health.HealthHandler)
	mux.HandleFunc("/ready", health.ReadyHandler)

	// Ingestion pipeline observability, Prometheus text exposition
	// format — see services/document-service/metrics.
	mux.HandleFunc("/metrics", metrics.Default.Handler())

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

	// Stop the background worker before shutting down the HTTP server —
	// no new requests to enqueue more ingestions, and RunLoop returns
	// promptly since it selects on ctx.Done() between passes.
	workerCancel()

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

// askHandler dispatches requests to /documents/ask by HTTP method.
func askHandler(h *handler.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			h.Ask(w, r)
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
