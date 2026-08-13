package worker

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vamshireddy02/mindova/services/document-service/embedding"
	"github.com/vamshireddy02/mindova/services/document-service/model"
	"github.com/vamshireddy02/mindova/services/document-service/repository"
	"github.com/vamshireddy02/mindova/services/document-service/service"
)

// openIntegrationDB connects to TEST_DATABASE_URL, skipping the test if
// unset — the same opt-in mechanism the repository package's integration
// tests use. The returned cleanup func closes the pool.
func openIntegrationDB(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	return pool, pool.Close
}

// TestWorker_EndToEnd_IngestionWithEmbeddings wires every real component —
// PostgreSQL-backed repositories, services, the real TextProcessor, and a
// MockEmbedder (the only non-real piece, since we're not calling an actual
// AI provider in tests) — and proves the full pipeline works end to end:
//
//	pending ingestion -> worker.Run() -> completed ingestion,
//	with chunks and their embeddings actually persisted in PostgreSQL.
func TestWorker_EndToEnd_IngestionWithEmbeddings(t *testing.T) {
	pool, cleanup := openIntegrationDB(t)
	defer cleanup()

	ctx := context.Background()

	// --- Real repositories -------------------------------------------------
	docRepo := repository.New(pool)
	ingestionRepo := repository.NewIngestionRepo(pool)
	chunkRepo := repository.NewChunkRepo(pool)

	// --- Real services -------------------------------------------------------
	documentService := service.New(docRepo)
	ingestionService := service.NewIngestionService(ingestionRepo)

	// --- Real processor, mock embedder ----------------------------------------
	processor := NewTextProcessor(0) // default chunk size
	embedder := &embedding.MockEmbedder{}

	// --- The worker under test -------------------------------------------------
	w := New(ingestionService, documentService, processor, embedder, chunkRepo, 0)

	// 1 & 2: create a real test document.
	doc := &model.Document{
		Name:        "e2e-ingestion-test.md",
		Content:     "Mindova is an AI knowledge platform. This document exists to prove the full ingestion pipeline works end to end, from a pending ingestion all the way through chunking, embedding, and PostgreSQL persistence.",
		ContentType: "text/markdown",
	}
	if err := documentService.Create(ctx, doc); err != nil {
		t.Fatalf("failed to create test document: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", doc.ID)

	// 3: create a pending ingestion for that document.
	ing := &model.Ingestion{DocumentID: doc.ID}
	if err := ingestionService.Create(ctx, ing); err != nil {
		t.Fatalf("failed to create ingestion: %v", err)
	}
	if ing.Status != model.IngestionPending {
		t.Fatalf("expected newly created ingestion to be pending, got %s", ing.Status)
	}

	// 5: run the worker.
	if err := w.Run(ctx); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	// 6: assert the ingestion completed.
	finished, err := ingestionService.GetByID(ctx, ing.ID)
	if err != nil {
		t.Fatalf("failed to fetch ingestion after Run(): %v", err)
	}
	if finished.Status != model.IngestionCompleted {
		t.Fatalf("expected ingestion status completed, got %s (error: %q)", finished.Status, finished.Error)
	}
	if finished.StartedAt == nil {
		t.Error("expected StartedAt to be set after processing")
	}
	if finished.CompletedAt == nil {
		t.Error("expected CompletedAt to be set after completion")
	}

	// Important assertion: read chunks back from PostgreSQL — not from
	// anything held in memory during Run() — to prove persistence
	// actually happened, not just that the worker's in-process view
	// looked correct.
	chunks, err := chunkRepo.GetByDocumentID(ctx, doc.ID)
	if err != nil {
		t.Fatalf("GetByDocumentID() returned error: %v", err)
	}

	if len(chunks) == 0 {
		t.Fatal("expected chunks")
	}

	for i, chunk := range chunks {
		if len(chunk.Embedding) != 8 {
			t.Fatalf(
				"chunk %d: expected embedding dimension 8, got %d",
				i, len(chunk.Embedding),
			)
		}
		if chunk.ChunkIndex != i {
			t.Errorf("chunk at position %d: expected ChunkIndex %d, got %d", i, i, chunk.ChunkIndex)
		}
		if chunk.DocumentID != doc.ID {
			t.Errorf("chunk %d: expected document_id %s, got %s", i, doc.ID, chunk.DocumentID)
		}
		if chunk.Content == "" {
			t.Errorf("chunk %d: expected non-empty content", i)
		}
	}
}
