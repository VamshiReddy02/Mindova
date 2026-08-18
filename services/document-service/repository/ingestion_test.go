package repository

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/vamshireddy02/mindova/services/document-service/model"
)

// createTestDocument is a small helper for ingestion tests: ingestions have
// a foreign key to documents, so every ingestion test needs one to exist
// first. Deleting the returned document cascades to delete its ingestions
// too (ON DELETE CASCADE), so a single deferred cleanup call covers both.
func createTestDocument(t *testing.T, ctx context.Context, repo *Repository) *model.Document {
	t.Helper()

	doc := &model.Document{
		Name:        "ingestion-test-doc.md",
		Content:     "content for ingestion tests",
		ContentType: "text/markdown",
	}
	if err := repo.Create(ctx, doc); err != nil {
		t.Fatalf("failed to create test document: %v", err)
	}
	return doc
}

func TestIngestionCreate(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	ctx := context.Background()
	docRepo := New(pool)
	ingRepo := NewIngestionRepo(pool)

	doc := createTestDocument(t, ctx, docRepo)
	defer pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", doc.ID)

	ing := &model.Ingestion{
		DocumentID: doc.ID,
		Status:     model.IngestionPending,
	}

	if err := ingRepo.Create(ctx, ing); err != nil {
		t.Fatalf("Create() returned error: %v", err)
	}

	if ing.ID == "" {
		t.Error("expected ID to be populated by PostgreSQL, got empty string")
	}
	if ing.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be populated by PostgreSQL, got zero value")
	}
	if ing.DocumentID != doc.ID {
		t.Errorf("expected document_id %s, got %s", doc.ID, ing.DocumentID)
	}
	if ing.Status != model.IngestionPending {
		t.Errorf("expected status pending, got %s", ing.Status)
	}
}

func TestIngestionGetByID(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	ctx := context.Background()
	docRepo := New(pool)
	ingRepo := NewIngestionRepo(pool)

	doc := createTestDocument(t, ctx, docRepo)
	defer pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", doc.ID)

	created := &model.Ingestion{
		DocumentID: doc.ID,
		Status:     model.IngestionPending,
	}
	if err := ingRepo.Create(ctx, created); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	fetched, err := ingRepo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID() returned error: %v", err)
	}

	if fetched.ID != created.ID {
		t.Errorf("expected ID %s, got %s", created.ID, fetched.ID)
	}
	if fetched.DocumentID != doc.ID {
		t.Errorf("expected document_id %s, got %s", doc.ID, fetched.DocumentID)
	}
	if fetched.Status != model.IngestionPending {
		t.Errorf("expected status pending, got %s", fetched.Status)
	}
	if fetched.Error != "" {
		t.Errorf("expected empty error, got %q", fetched.Error)
	}
	if fetched.StartedAt != nil {
		t.Errorf("expected nil StartedAt for a pending ingestion, got %v", fetched.StartedAt)
	}
	if fetched.CompletedAt != nil {
		t.Errorf("expected nil CompletedAt for a pending ingestion, got %v", fetched.CompletedAt)
	}
}

func TestIngestionGetByID_NotFound(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	ingRepo := NewIngestionRepo(pool)
	ctx := context.Background()

	_, err := ingRepo.GetByID(ctx, "00000000-0000-0000-0000-000000000000")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestIngestionGetByDocumentID(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	ctx := context.Background()
	docRepo := New(pool)
	ingRepo := NewIngestionRepo(pool)

	doc := createTestDocument(t, ctx, docRepo)
	defer pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", doc.ID)

	// Simulate a document with a history of ingestion attempts:
	// two failures followed by a success.
	attempt1 := &model.Ingestion{DocumentID: doc.ID, Status: model.IngestionFailed, Error: "first failure"}
	attempt2 := &model.Ingestion{DocumentID: doc.ID, Status: model.IngestionFailed, Error: "second failure"}
	attempt3 := &model.Ingestion{DocumentID: doc.ID, Status: model.IngestionCompleted}

	for _, ing := range []*model.Ingestion{attempt1, attempt2, attempt3} {
		if err := ingRepo.Create(ctx, ing); err != nil {
			t.Fatalf("Create() failed: %v", err)
		}
	}

	history, err := ingRepo.GetByDocumentID(ctx, doc.ID)
	if err != nil {
		t.Fatalf("GetByDocumentID() returned error: %v", err)
	}

	if len(history) != 3 {
		t.Fatalf("expected 3 ingestion attempts, got %d", len(history))
	}

	// Every returned ingestion should belong to this document.
	for _, ing := range history {
		if ing.DocumentID != doc.ID {
			t.Errorf("expected document_id %s, got %s", doc.ID, ing.DocumentID)
		}
	}
}

func TestIngestionGetByDocumentID_Empty(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	ctx := context.Background()
	docRepo := New(pool)
	ingRepo := NewIngestionRepo(pool)

	doc := createTestDocument(t, ctx, docRepo)
	defer pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", doc.ID)

	history, err := ingRepo.GetByDocumentID(ctx, doc.ID)
	if err != nil {
		t.Fatalf("GetByDocumentID() returned error: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("expected no ingestion attempts, got %d", len(history))
	}
}

func TestIngestionUpdateStatus_ToProcessing(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	ctx := context.Background()
	docRepo := New(pool)
	ingRepo := NewIngestionRepo(pool)

	doc := createTestDocument(t, ctx, docRepo)
	defer pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", doc.ID)

	ing := &model.Ingestion{DocumentID: doc.ID, Status: model.IngestionPending}
	if err := ingRepo.Create(ctx, ing); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	if err := ingRepo.UpdateStatus(ctx, ing.ID, model.IngestionProcessing, ""); err != nil {
		t.Fatalf("UpdateStatus() returned error: %v", err)
	}

	fetched, err := ingRepo.GetByID(ctx, ing.ID)
	if err != nil {
		t.Fatalf("GetByID() after update failed: %v", err)
	}

	if fetched.Status != model.IngestionProcessing {
		t.Errorf("expected status processing, got %s", fetched.Status)
	}
	if fetched.StartedAt == nil {
		t.Error("expected StartedAt to be set after transitioning to processing")
	}
	if fetched.CompletedAt != nil {
		t.Error("expected CompletedAt to remain nil while processing")
	}
}

func TestIngestionUpdateStatus_ToCompleted(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	ctx := context.Background()
	docRepo := New(pool)
	ingRepo := NewIngestionRepo(pool)

	doc := createTestDocument(t, ctx, docRepo)
	defer pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", doc.ID)

	ing := &model.Ingestion{DocumentID: doc.ID, Status: model.IngestionPending}
	if err := ingRepo.Create(ctx, ing); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	if err := ingRepo.UpdateStatus(ctx, ing.ID, model.IngestionProcessing, ""); err != nil {
		t.Fatalf("UpdateStatus() to processing failed: %v", err)
	}
	if err := ingRepo.UpdateStatus(ctx, ing.ID, model.IngestionCompleted, ""); err != nil {
		t.Fatalf("UpdateStatus() to completed failed: %v", err)
	}

	fetched, err := ingRepo.GetByID(ctx, ing.ID)
	if err != nil {
		t.Fatalf("GetByID() after update failed: %v", err)
	}

	if fetched.Status != model.IngestionCompleted {
		t.Errorf("expected status completed, got %s", fetched.Status)
	}
	if fetched.StartedAt == nil {
		t.Error("expected StartedAt to remain set from the processing transition")
	}
	if fetched.CompletedAt == nil {
		t.Error("expected CompletedAt to be set after transitioning to completed")
	}
	if fetched.Error != "" {
		t.Errorf("expected empty error on success, got %q", fetched.Error)
	}
}

func TestIngestionUpdateStatus_ToFailed(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	ctx := context.Background()
	docRepo := New(pool)
	ingRepo := NewIngestionRepo(pool)

	doc := createTestDocument(t, ctx, docRepo)
	defer pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", doc.ID)

	ing := &model.Ingestion{DocumentID: doc.ID, Status: model.IngestionPending}
	if err := ingRepo.Create(ctx, ing); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	if err := ingRepo.UpdateStatus(ctx, ing.ID, model.IngestionProcessing, ""); err != nil {
		t.Fatalf("UpdateStatus() to processing failed: %v", err)
	}
	if err := ingRepo.UpdateStatus(ctx, ing.ID, model.IngestionFailed, "embedding service timed out"); err != nil {
		t.Fatalf("UpdateStatus() to failed failed: %v", err)
	}

	fetched, err := ingRepo.GetByID(ctx, ing.ID)
	if err != nil {
		t.Fatalf("GetByID() after update failed: %v", err)
	}

	if fetched.Status != model.IngestionFailed {
		t.Errorf("expected status failed, got %s", fetched.Status)
	}
	if fetched.CompletedAt == nil {
		t.Error("expected CompletedAt to be set after transitioning to failed")
	}
	if fetched.Error != "embedding service timed out" {
		t.Errorf("expected error message to be stored, got %q", fetched.Error)
	}
}

func TestIngestionUpdateStatus_NotFound(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	ingRepo := NewIngestionRepo(pool)
	ctx := context.Background()

	err := ingRepo.UpdateStatus(ctx, "00000000-0000-0000-0000-000000000000", model.IngestionProcessing, "")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestIngestionListPending(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	ctx := context.Background()
	docRepo := New(pool)
	ingRepo := NewIngestionRepo(pool)

	doc := createTestDocument(t, ctx, docRepo)
	defer pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", doc.ID)

	// One pending, one already completed, one already failed.
	// Only the pending one should come back.
	pendingIng := &model.Ingestion{DocumentID: doc.ID, Status: model.IngestionPending}
	completedIng := &model.Ingestion{DocumentID: doc.ID, Status: model.IngestionCompleted}
	failedIng := &model.Ingestion{DocumentID: doc.ID, Status: model.IngestionFailed}

	for _, ing := range []*model.Ingestion{pendingIng, completedIng, failedIng} {
		if err := ingRepo.Create(ctx, ing); err != nil {
			t.Fatalf("Create() failed: %v", err)
		}
	}

	pending, err := ingRepo.ListPending(ctx, 10)
	if err != nil {
		t.Fatalf("ListPending() returned error: %v", err)
	}

	if len(pending) != 1 {
		t.Fatalf("expected 1 pending ingestion, got %d", len(pending))
	}
	if pending[0].ID != pendingIng.ID {
		t.Errorf("expected pending ingestion %s, got %s", pendingIng.ID, pending[0].ID)
	}
	if pending[0].Status != model.IngestionPending {
		t.Errorf("expected status pending, got %s", pending[0].Status)
	}
}

func TestIngestionListPending_RespectsLimit(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	ctx := context.Background()
	docRepo := New(pool)
	ingRepo := NewIngestionRepo(pool)

	doc := createTestDocument(t, ctx, docRepo)
	defer pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", doc.ID)

	for i := 0; i < 3; i++ {
		ing := &model.Ingestion{DocumentID: doc.ID, Status: model.IngestionPending}
		if err := ingRepo.Create(ctx, ing); err != nil {
			t.Fatalf("Create() failed: %v", err)
		}
	}

	pending, err := ingRepo.ListPending(ctx, 2)
	if err != nil {
		t.Fatalf("ListPending() returned error: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("expected limit to cap result at 2, got %d", len(pending))
	}
}

func TestIngestionListPending_Empty(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	ingRepo := NewIngestionRepo(pool)
	ctx := context.Background()

	pending, err := ingRepo.ListPending(ctx, 10)
	if err != nil {
		t.Fatalf("ListPending() returned error: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("expected no pending ingestions in a fresh table state, got %d (this test assumes no leftover pending rows from other tests)", len(pending))
	}
}

// --- ClaimNextPending ------------------------------------------------

func TestIngestionClaimNextPending_ClaimsOldestPending(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	ctx := context.Background()
	docRepo := New(pool)
	ingRepo := NewIngestionRepo(pool)

	doc := createTestDocument(t, ctx, docRepo)
	defer pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", doc.ID)

	first := &model.Ingestion{DocumentID: doc.ID, Status: model.IngestionPending}
	if err := ingRepo.Create(ctx, first); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	second := &model.Ingestion{DocumentID: doc.ID, Status: model.IngestionPending}
	if err := ingRepo.Create(ctx, second); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	claimed, err := ingRepo.ClaimNextPending(ctx)
	if err != nil {
		t.Fatalf("ClaimNextPending() returned error: %v", err)
	}
	if claimed == nil {
		t.Fatal("expected a claimed ingestion, got nil")
	}

	if claimed.ID != first.ID {
		t.Errorf("expected the oldest pending ingestion (%s) to be claimed, got %s", first.ID, claimed.ID)
	}
	if claimed.Status != model.IngestionProcessing {
		t.Errorf("expected claimed ingestion status processing, got %s", claimed.Status)
	}
	if claimed.Attempts != 1 {
		t.Errorf("expected attempts incremented to 1, got %d", claimed.Attempts)
	}
	if claimed.StartedAt == nil {
		t.Error("expected StartedAt to be set on claim")
	}
	if claimed.ProcessedAt == nil {
		t.Error("expected ProcessedAt to be set on claim")
	}
}

func TestIngestionClaimNextPending_SkipsNonPending(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	ctx := context.Background()
	docRepo := New(pool)
	ingRepo := NewIngestionRepo(pool)

	doc := createTestDocument(t, ctx, docRepo)
	defer pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", doc.ID)

	completed := &model.Ingestion{DocumentID: doc.ID, Status: model.IngestionCompleted}
	if err := ingRepo.Create(ctx, completed); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	pending := &model.Ingestion{DocumentID: doc.ID, Status: model.IngestionPending}
	if err := ingRepo.Create(ctx, pending); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	claimed, err := ingRepo.ClaimNextPending(ctx)
	if err != nil {
		t.Fatalf("ClaimNextPending() returned error: %v", err)
	}
	if claimed == nil {
		t.Fatal("expected a claimed ingestion, got nil")
	}
	if claimed.ID != pending.ID {
		t.Errorf("expected the pending ingestion to be claimed, got %s (status was %s before claim)", claimed.ID, completed.Status)
	}
}

func TestIngestionClaimNextPending_NoneAvailable(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	ctx := context.Background()
	docRepo := New(pool)
	ingRepo := NewIngestionRepo(pool)

	doc := createTestDocument(t, ctx, docRepo)
	defer pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", doc.ID)

	// Only a completed ingestion exists — nothing pending to claim.
	completed := &model.Ingestion{DocumentID: doc.ID, Status: model.IngestionCompleted}
	if err := ingRepo.Create(ctx, completed); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	claimed, err := ingRepo.ClaimNextPending(ctx)
	if err != nil {
		t.Fatalf("ClaimNextPending() returned error: %v", err)
	}
	if claimed != nil {
		t.Errorf("expected nil when nothing is pending, got %+v", claimed)
	}
}

func TestIngestionClaimNextPending_ConcurrentClaimsDoNotDuplicate(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	ctx := context.Background()
	docRepo := New(pool)
	ingRepo := NewIngestionRepo(pool)

	doc := createTestDocument(t, ctx, docRepo)
	defer pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", doc.ID)

	const numIngestions = 10
	ids := make(map[string]bool, numIngestions)
	for i := 0; i < numIngestions; i++ {
		ing := &model.Ingestion{DocumentID: doc.ID, Status: model.IngestionPending}
		if err := ingRepo.Create(ctx, ing); err != nil {
			t.Fatalf("Create() failed: %v", err)
		}
		ids[ing.ID] = true
	}

	// This is the real proof that FOR UPDATE SKIP LOCKED works: fire off
	// many concurrent claimers against real PostgreSQL and verify every
	// pending ingestion was claimed by exactly one of them — no row
	// claimed twice, none claimed zero times.
	const numWorkers = 5
	var wg sync.WaitGroup
	var mu sync.Mutex
	claimedIDs := make(map[string]int) // id -> number of times claimed

	claimOne := func() {
		defer wg.Done()
		claimed, err := ingRepo.ClaimNextPending(ctx)
		if err != nil {
			t.Errorf("ClaimNextPending() returned error: %v", err)
			return
		}
		if claimed == nil {
			return
		}
		mu.Lock()
		claimedIDs[claimed.ID]++
		mu.Unlock()
	}

	// Launch more claim attempts than there are ingestions, so some
	// workers legitimately get nil (nothing left) — that's expected and
	// fine; the assertion is about duplicates, not about every launch
	// claiming something.
	for i := 0; i < numWorkers*3; i++ {
		wg.Add(1)
		go claimOne()
	}
	wg.Wait()

	if len(claimedIDs) != numIngestions {
		t.Errorf("expected all %d ingestions claimed exactly once, got %d distinct ingestions claimed", numIngestions, len(claimedIDs))
	}
	for id, count := range claimedIDs {
		if count != 1 {
			t.Errorf("ingestion %s was claimed %d times, expected exactly 1 — FOR UPDATE SKIP LOCKED should prevent this", id, count)
		}
		if !ids[id] {
			t.Errorf("claimed an ingestion %s that wasn't one of ours", id)
		}
	}
}
