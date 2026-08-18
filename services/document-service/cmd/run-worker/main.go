package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/vamshireddy02/mindova/packages/kernel/config"

	"github.com/vamshireddy02/mindova/services/document-service/database"
	"github.com/vamshireddy02/mindova/services/document-service/embedding"
	"github.com/vamshireddy02/mindova/services/document-service/repository"
	"github.com/vamshireddy02/mindova/services/document-service/service"
	"github.com/vamshireddy02/mindova/services/document-service/worker"
)

// Standalone command: connects to PostgreSQL, wires up the real worker
// (real repositories, real services, real TextProcessor, real
// HTTPEmbedder calling the Python AI service), and runs one batch of
// pending ingestions to completion.
//
// This exists as a manual one-off trigger for testing the pipeline
// against real data. The running document-service process now runs its
// own continuous background worker (see cmd/document-service/main.go),
// so this is no longer required for normal operation.
//
// Run with:
//
//	export APP_NAME=run-worker
//	export DB_PORT=5433
//	export EMBEDDING_SERVICE_URL=http://localhost:8001
//	export MAX_ATTEMPTS=3
//	go run ./services/document-service/cmd/run-worker
func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		os.Exit(1)
	}

	connectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	db, err := database.New(connectCtx, cfg.Database)
	cancel()
	if err != nil {
		fmt.Fprintln(os.Stderr, "database connection failed:", err)
		os.Exit(1)
	}
	defer db.Close()

	docRepo := repository.New(db)
	ingestionRepo := repository.NewIngestionRepo(db)
	chunkRepo := repository.NewChunkRepo(db)

	documentService := service.New(docRepo)
	ingestionService := service.NewIngestionService(ingestionRepo)

	processor := worker.NewTextProcessor(0)

	embeddingServiceURL := os.Getenv("EMBEDDING_SERVICE_URL")
	if embeddingServiceURL == "" {
		embeddingServiceURL = "http://localhost:8001"
	}
	embedder := embedding.NewHTTPEmbedder(embeddingServiceURL, nil)

	fmt.Println("using embedding service:", embeddingServiceURL)

	maxAttempts := 3
	if raw := os.Getenv("MAX_ATTEMPTS"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			maxAttempts = parsed
		} else {
			fmt.Fprintf(os.Stderr, "invalid MAX_ATTEMPTS %q, using default %d\n", raw, maxAttempts)
		}
	}

	w := worker.New(ingestionService, documentService, processor, embedder, chunkRepo, 0, maxAttempts)

	fmt.Println("running worker...")

	runCtx, runCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer runCancel()

	if err := w.Run(runCtx); err != nil {
		fmt.Fprintln(os.Stderr, "worker.Run() failed:", err)
		os.Exit(1)
	}

	fmt.Println("worker run complete")
}
