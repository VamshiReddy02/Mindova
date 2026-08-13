package repository

import (
	"context"
	"testing"

	"github.com/pgvector/pgvector-go"

	"github.com/vamshireddy02/mindova/services/document-service/model"
)

// testEmbedding returns a valid 8-dimension embedding (matching the
// document_chunks.embedding vector(8) column) seeded from a float so
// tests can produce distinguishable-but-valid vectors easily.
func testEmbedding(seed float32) []float32 {
	return []float32{seed, seed, seed, seed, seed, seed, seed, seed}
}

func TestChunkCreateBatch(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	ctx := context.Background()
	docRepo := New(pool)
	chunkRepo := NewChunkRepo(pool)

	doc := createTestDocument(t, ctx, docRepo)
	defer pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", doc.ID)

	chunks := []*model.DocumentChunk{
		{DocumentID: doc.ID, ChunkIndex: 0, Content: "first chunk", Embedding: testEmbedding(0.1)},
		{DocumentID: doc.ID, ChunkIndex: 1, Content: "second chunk", Embedding: testEmbedding(0.2)},
		{DocumentID: doc.ID, ChunkIndex: 2, Content: "third chunk", Embedding: testEmbedding(0.3)},
	}

	if err := chunkRepo.CreateBatch(ctx, chunks); err != nil {
		t.Fatalf("CreateBatch() returned error: %v", err)
	}

	for i, c := range chunks {
		if c.ID == "" {
			t.Errorf("chunk %d: expected ID to be populated, got empty string", i)
		}
		if c.CreatedAt.IsZero() {
			t.Errorf("chunk %d: expected CreatedAt to be populated, got zero value", i)
		}
	}
}

func TestChunkCreateBatch_WithEmbeddings(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	ctx := context.Background()
	docRepo := New(pool)
	chunkRepo := NewChunkRepo(pool)

	doc := createTestDocument(t, ctx, docRepo)
	defer pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", doc.ID)

	wantEmbedding := []float32{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8}

	chunks := []*model.DocumentChunk{
		{DocumentID: doc.ID, ChunkIndex: 0, Content: "chunk with embedding", Embedding: wantEmbedding},
	}

	if err := chunkRepo.CreateBatch(ctx, chunks); err != nil {
		t.Fatalf("CreateBatch() returned error: %v", err)
	}

	// Read the raw column back directly (bypassing GetByDocumentID) to
	// prove the embedding actually landed in PostgreSQL, not just that
	// our own Go struct still holds the value we set.
	var vec pgvector.Vector
	err := pool.QueryRow(ctx, "SELECT embedding FROM document_chunks WHERE id = $1", chunks[0].ID).Scan(&vec)
	if err != nil {
		t.Fatalf("failed to read back embedding column: %v", err)
	}
	raw := vec.Slice()

	if len(raw) != len(wantEmbedding) {
		t.Fatalf("expected stored embedding of length %d, got %d", len(wantEmbedding), len(raw))
	}
	for i := range wantEmbedding {
		if raw[i] != wantEmbedding[i] {
			t.Errorf("dimension %d: expected %v, got %v", i, wantEmbedding[i], raw[i])
		}
	}
}

func TestChunkCreateBatch_EmbeddingDimensionMismatch(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	ctx := context.Background()
	docRepo := New(pool)
	chunkRepo := NewChunkRepo(pool)

	doc := createTestDocument(t, ctx, docRepo)
	defer pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", doc.ID)

	chunks := []*model.DocumentChunk{
		{DocumentID: doc.ID, ChunkIndex: 0, Content: "bad embedding", Embedding: []float32{0.1, 0.2, 0.3}}, // only 3 dims, not 8
	}

	err := chunkRepo.CreateBatch(ctx, chunks)
	if err == nil {
		t.Fatal("expected error for mismatched embedding dimensions, got nil")
	}

	// Nothing should have been persisted — this is a validation failure
	// caught before any database round-trip, and CreateBatch's earlier
	// all-or-nothing guarantee should hold too.
	stored, getErr := chunkRepo.GetByDocumentID(ctx, doc.ID)
	if getErr != nil {
		t.Fatalf("GetByDocumentID() returned error: %v", getErr)
	}
	if len(stored) != 0 {
		t.Errorf("expected no chunks persisted after a dimension-mismatch rejection, got %d", len(stored))
	}
}

func TestChunkCreateBatch_Empty(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	chunkRepo := NewChunkRepo(pool)
	ctx := context.Background()

	if err := chunkRepo.CreateBatch(ctx, nil); err != nil {
		t.Fatalf("CreateBatch(nil) should be a no-op, got error: %v", err)
	}
	if err := chunkRepo.CreateBatch(ctx, []*model.DocumentChunk{}); err != nil {
		t.Fatalf("CreateBatch(empty slice) should be a no-op, got error: %v", err)
	}
}

func TestChunkCreateBatch_DuplicateIndexRejected(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	ctx := context.Background()
	docRepo := New(pool)
	chunkRepo := NewChunkRepo(pool)

	doc := createTestDocument(t, ctx, docRepo)
	defer pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", doc.ID)

	chunks := []*model.DocumentChunk{
		{DocumentID: doc.ID, ChunkIndex: 0, Content: "first", Embedding: testEmbedding(0.1)},
		{DocumentID: doc.ID, ChunkIndex: 0, Content: "duplicate index", Embedding: testEmbedding(0.2)}, // same index, should violate unique constraint
	}

	err := chunkRepo.CreateBatch(ctx, chunks)
	if err == nil {
		t.Fatal("expected error inserting duplicate (document_id, chunk_index), got nil")
	}

	// Because CreateBatch runs in a transaction, the first chunk must not
	// have been persisted either — the batch is all-or-nothing.
	stored, getErr := chunkRepo.GetByDocumentID(ctx, doc.ID)
	if getErr != nil {
		t.Fatalf("GetByDocumentID() returned error: %v", getErr)
	}
	if len(stored) != 0 {
		t.Errorf("expected no chunks persisted after a failed batch, got %d", len(stored))
	}
}

func TestChunkGetByDocumentID_OrderedByIndex(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	ctx := context.Background()
	docRepo := New(pool)
	chunkRepo := NewChunkRepo(pool)

	doc := createTestDocument(t, ctx, docRepo)
	defer pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", doc.ID)

	// Insert out of order; GetByDocumentID should still return them
	// ordered by chunk_index, not insertion order.
	chunks := []*model.DocumentChunk{
		{DocumentID: doc.ID, ChunkIndex: 2, Content: "third", Embedding: testEmbedding(0.3)},
		{DocumentID: doc.ID, ChunkIndex: 0, Content: "first", Embedding: testEmbedding(0.1)},
		{DocumentID: doc.ID, ChunkIndex: 1, Content: "second", Embedding: testEmbedding(0.2)},
	}
	if err := chunkRepo.CreateBatch(ctx, chunks); err != nil {
		t.Fatalf("CreateBatch() returned error: %v", err)
	}

	got, err := chunkRepo.GetByDocumentID(ctx, doc.ID)
	if err != nil {
		t.Fatalf("GetByDocumentID() returned error: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(got))
	}

	wantOrder := []string{"first", "second", "third"}
	for i, c := range got {
		if c.Content != wantOrder[i] {
			t.Errorf("position %d: expected content %q, got %q", i, wantOrder[i], c.Content)
		}
		if c.ChunkIndex != i {
			t.Errorf("position %d: expected chunk_index %d, got %d", i, i, c.ChunkIndex)
		}
	}
}

func TestChunkGetByDocumentID_ReturnsEmbeddings(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	ctx := context.Background()
	docRepo := New(pool)
	chunkRepo := NewChunkRepo(pool)

	doc := createTestDocument(t, ctx, docRepo)
	defer pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", doc.ID)

	wantEmbeddings := [][]float32{
		{0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1},
		{0.9, 0.9, 0.9, 0.9, 0.9, 0.9, 0.9, 0.9},
	}

	chunks := []*model.DocumentChunk{
		{DocumentID: doc.ID, ChunkIndex: 0, Content: "first", Embedding: wantEmbeddings[0]},
		{DocumentID: doc.ID, ChunkIndex: 1, Content: "second", Embedding: wantEmbeddings[1]},
	}
	if err := chunkRepo.CreateBatch(ctx, chunks); err != nil {
		t.Fatalf("CreateBatch() returned error: %v", err)
	}

	got, err := chunkRepo.GetByDocumentID(ctx, doc.ID)
	if err != nil {
		t.Fatalf("GetByDocumentID() returned error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(got))
	}

	for i, c := range got {
		if len(c.Embedding) != 8 {
			t.Fatalf("chunk %d: expected embedding of length 8, got %d", i, len(c.Embedding))
		}
		for d := range wantEmbeddings[i] {
			if c.Embedding[d] != wantEmbeddings[i][d] {
				t.Errorf("chunk %d, dimension %d: expected %v, got %v", i, d, wantEmbeddings[i][d], c.Embedding[d])
			}
		}
	}
}

func TestChunkGetByDocumentID_Empty(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	ctx := context.Background()
	docRepo := New(pool)
	chunkRepo := NewChunkRepo(pool)

	doc := createTestDocument(t, ctx, docRepo)
	defer pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", doc.ID)

	got, err := chunkRepo.GetByDocumentID(ctx, doc.ID)
	if err != nil {
		t.Fatalf("GetByDocumentID() returned error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no chunks for a document with none created, got %d", len(got))
	}
}

func TestChunkCascadeDeletesWithDocument(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	ctx := context.Background()
	docRepo := New(pool)
	chunkRepo := NewChunkRepo(pool)

	doc := createTestDocument(t, ctx, docRepo)

	chunks := []*model.DocumentChunk{
		{DocumentID: doc.ID, ChunkIndex: 0, Content: "chunk one", Embedding: testEmbedding(0.1)},
		{DocumentID: doc.ID, ChunkIndex: 1, Content: "chunk two", Embedding: testEmbedding(0.2)},
	}
	if err := chunkRepo.CreateBatch(ctx, chunks); err != nil {
		t.Fatalf("CreateBatch() returned error: %v", err)
	}

	// Deleting the document should cascade-delete its chunks (ON DELETE
	// CASCADE in the migration).
	if _, err := pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", doc.ID); err != nil {
		t.Fatalf("failed to delete document: %v", err)
	}

	remaining, err := chunkRepo.GetByDocumentID(ctx, doc.ID)
	if err != nil {
		t.Fatalf("GetByDocumentID() returned error: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("expected chunks to be cascade-deleted with their document, got %d remaining", len(remaining))
	}
}
