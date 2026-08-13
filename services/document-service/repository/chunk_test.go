package repository

import (
	"context"
	"testing"

	"github.com/vamshireddy02/mindova/services/document-service/model"
)

func TestChunkCreateBatch(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	ctx := context.Background()
	docRepo := New(pool)
	chunkRepo := NewChunkRepo(pool)

	doc := createTestDocument(t, ctx, docRepo)
	defer pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", doc.ID)

	chunks := []*model.DocumentChunk{
		{DocumentID: doc.ID, ChunkIndex: 0, Content: "first chunk"},
		{DocumentID: doc.ID, ChunkIndex: 1, Content: "second chunk"},
		{DocumentID: doc.ID, ChunkIndex: 2, Content: "third chunk"},
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
		{DocumentID: doc.ID, ChunkIndex: 0, Content: "first"},
		{DocumentID: doc.ID, ChunkIndex: 0, Content: "duplicate index"}, // same index, should violate unique constraint
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
		{DocumentID: doc.ID, ChunkIndex: 2, Content: "third"},
		{DocumentID: doc.ID, ChunkIndex: 0, Content: "first"},
		{DocumentID: doc.ID, ChunkIndex: 1, Content: "second"},
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
		{DocumentID: doc.ID, ChunkIndex: 0, Content: "chunk one"},
		{DocumentID: doc.ID, ChunkIndex: 1, Content: "chunk two"},
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
