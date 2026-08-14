package service

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vamshireddy02/mindova/services/document-service/embedding"
	"github.com/vamshireddy02/mindova/services/document-service/llm"
	"github.com/vamshireddy02/mindova/services/document-service/model"
	"github.com/vamshireddy02/mindova/services/document-service/repository"
)

// openRAGIntegrationDB connects to TEST_DATABASE_URL, skipping the test
// if unset — same opt-in mechanism used throughout the repository
// package's integration tests.
func openRAGIntegrationDB(t *testing.T) (*pgxpool.Pool, func()) {
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

// aiServiceBaseURL returns the Python AI service's base URL, defaulting
// to the same localhost:8001 used throughout local development.
func aiServiceBaseURL() string {
	if url := os.Getenv("AI_SERVICE_URL"); url != "" {
		return url
	}
	return "http://localhost:8001"
}

// requireAIService skips the test if the AI service isn't reachable,
// rather than failing with a confusing connection-refused error deep
// inside the test body.
func requireAIService(t *testing.T, baseURL string) {
	t.Helper()

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(baseURL + "/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Skipf(
			"AI service not reachable at %s, skipping integration test "+
				"(start it with: cd services/ai-service && uvicorn app.main:app --port 8001)",
			baseURL,
		)
	}
	resp.Body.Close()
}

// TestRAGService_EndToEnd_AskWithRealServices wires every real
// component — PostgreSQL-backed repositories, the real HTTPEmbedder and
// llm.HTTPClient calling an actually-running Python AI service — and
// proves the full RAG loop works end to end:
//
//	question -> embed -> pgvector search -> retrieved chunk -> LLM -> answer
//
// Unlike the mock-based tests in rag_test.go, nothing here is faked:
// this is the real HTTP round trip to Python for both embedding and
// generation, against a real PostgreSQL instance.
func TestRAGService_EndToEnd_AskWithRealServices(t *testing.T) {
	pool, cleanup := openRAGIntegrationDB(t)
	defer cleanup()

	baseURL := aiServiceBaseURL()
	requireAIService(t, baseURL)

	ctx := context.Background()

	docRepo := repository.New(pool)
	chunkRepo := repository.NewChunkRepo(pool)

	doc := &model.Document{
		Name:        "rag-e2e-test.md",
		Content:     "placeholder — the real content lives in the chunk below",
		ContentType: "text/markdown",
	}
	if err := docRepo.Create(ctx, doc); err != nil {
		t.Fatalf("failed to create test document: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", doc.ID)

	embedder := embedding.NewHTTPEmbedder(baseURL, nil)

	// Embed the chunk's content through the real AI service, not a
	// hand-picked fake vector — the stored vector needs to live in the
	// same space the query will be embedded into, or this wouldn't prove
	// retrieval actually works, just that pgvector can store numbers.
	chunkText := "Mindova chunks documents, generates an embedding for each chunk, " +
		"and stores the resulting vectors in PostgreSQL using pgvector."
	vectors, err := embedder.Embed(ctx, []string{chunkText})
	if err != nil {
		t.Fatalf("failed to embed chunk content via AI service: %v", err)
	}

	chunk := &model.DocumentChunk{
		DocumentID: doc.ID,
		ChunkIndex: 0,
		Content:    chunkText,
		Embedding:  vectors[0],
	}
	if err := chunkRepo.CreateBatch(ctx, []*model.DocumentChunk{chunk}); err != nil {
		t.Fatalf("failed to persist chunk: %v", err)
	}

	retrievalSvc := NewRetrievalService(embedder, chunkRepo)
	llmClient := llm.NewHTTPClient(baseURL, nil)
	ragSvc := NewRAGService(retrievalSvc, llmClient)

	resp, err := ragSvc.Ask(ctx, "How does Mindova store embeddings?", 5)
	if err != nil {
		t.Fatalf("Ask() returned error: %v", err)
	}

	if resp.Answer == "" {
		t.Error("expected a non-empty answer from the AI service")
	}

	if len(resp.Chunks) == 0 {
		t.Fatal("expected at least one retrieved chunk")
	}

	found := false
	for _, c := range resp.Chunks {
		if c.DocumentID == doc.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected the retrieved chunks to include our test document's chunk")
	}
}
