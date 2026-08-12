package repository

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vamshireddy02/mindova/services/document-service/model"
)

// Compile-time check that *Repository satisfies DocumentRepository.
var _ DocumentRepository = (*Repository)(nil)

// TestCreate is an integration test that runs against a real PostgreSQL
// instance. It's skipped automatically unless TEST_DATABASE_URL is set,
// so `go test ./...` stays fast and safe in environments without a
// database available.
//
// Example:
//
//	export TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5433/mindova?sslmode=disable"
//	go test ./services/document-service/repository -v
func TestCreate(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	ctx := context.Background()
	repo := New(pool)

	doc := &model.Document{
		Name:        "test-document.md",
		Content:     "This is a test document created by an integration test.",
		ContentType: "text/markdown",
	}

	err := repo.Create(ctx, doc)
	if err != nil {
		t.Fatalf("Create() returned error: %v", err)
	}

	// PostgreSQL should have populated these
	if doc.ID == "" {
		t.Error("expected ID to be populated by PostgreSQL, got empty string")
	}
	if doc.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be populated by PostgreSQL, got zero value")
	}
	if doc.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be populated by PostgreSQL, got zero value")
	}

	// Fields we set should be unchanged
	if doc.Name != "test-document.md" {
		t.Errorf("expected name to remain test-document.md, got %s", doc.Name)
	}
	if doc.ContentType != "text/markdown" {
		t.Errorf("expected content_type to remain text/markdown, got %s", doc.ContentType)
	}

	// Cleanup: remove the row we just inserted
	_, err = pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", doc.ID)
	if err != nil {
		t.Logf("cleanup failed (non-fatal): %v", err)
	}
}

func TestGetByID(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	repo := New(pool)
	ctx := context.Background()

	created := &model.Document{
		Name:        "getbyid-test.md",
		Content:     "content for GetByID test",
		ContentType: "text/markdown",
	}
	if err := repo.Create(ctx, created); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", created.ID)

	fetched, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID() returned error: %v", err)
	}

	if fetched.ID != created.ID {
		t.Errorf("expected ID %s, got %s", created.ID, fetched.ID)
	}
	if fetched.Name != created.Name {
		t.Errorf("expected name %s, got %s", created.Name, fetched.Name)
	}
	if fetched.Content != created.Content {
		t.Errorf("expected content %s, got %s", created.Content, fetched.Content)
	}
}

func TestGetByID_NotFound(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	repo := New(pool)
	ctx := context.Background()

	_, err := repo.GetByID(ctx, "00000000-0000-0000-0000-000000000000")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestList(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	repo := New(pool)
	ctx := context.Background()

	doc1 := &model.Document{Name: "list-test-1.md", Content: "one", ContentType: "text/markdown"}
	doc2 := &model.Document{Name: "list-test-2.md", Content: "two", ContentType: "text/markdown"}

	if err := repo.Create(ctx, doc1); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", doc1.ID)

	if err := repo.Create(ctx, doc2); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", doc2.ID)

	docs, err := repo.List(ctx, 20)
	if err != nil {
		t.Fatalf("List() returned error: %v", err)
	}

	found1, found2 := false, false
	for _, d := range docs {
		if d.ID == doc1.ID {
			found1 = true
		}
		if d.ID == doc2.ID {
			found2 = true
		}
	}

	if !found1 || !found2 {
		t.Errorf("expected both created documents in List() result, found1=%v found2=%v", found1, found2)
	}
}

func TestUpdate(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	repo := New(pool)
	ctx := context.Background()

	doc := &model.Document{
		Name:        "update-test.md",
		Content:     "original content",
		ContentType: "text/markdown",
	}
	if err := repo.Create(ctx, doc); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", doc.ID)

	originalUpdatedAt := doc.UpdatedAt

	doc.Name = "update-test-renamed.md"
	doc.Content = "updated content"

	if err := repo.Update(ctx, doc); err != nil {
		t.Fatalf("Update() returned error: %v", err)
	}

	if !doc.UpdatedAt.After(originalUpdatedAt) {
		t.Errorf("expected UpdatedAt to advance, got %v (was %v)", doc.UpdatedAt, originalUpdatedAt)
	}

	fetched, err := repo.GetByID(ctx, doc.ID)
	if err != nil {
		t.Fatalf("GetByID() after update failed: %v", err)
	}
	if fetched.Name != "update-test-renamed.md" {
		t.Errorf("expected updated name, got %s", fetched.Name)
	}
	if fetched.Content != "updated content" {
		t.Errorf("expected updated content, got %s", fetched.Content)
	}
}

// TestUpdate_PreservesCreatedAt mirrors the handler's pattern of building a
// fresh model.Document (only ID, Name, Content, ContentType set — CreatedAt
// left as zero value) and passing it to Update(). This is a regression test
// for a bug where Update()'s query only returned updated_at, so CreatedAt
// on the returned doc stayed at Go's zero time instead of the real value.
func TestUpdate_PreservesCreatedAt(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	repo := New(pool)
	ctx := context.Background()

	// Create a document and remember its real CreatedAt
	created := &model.Document{
		Name:        "preserve-created-at.md",
		Content:     "original",
		ContentType: "text/markdown",
	}
	if err := repo.Create(ctx, created); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", created.ID)

	originalCreatedAt := created.CreatedAt

	// Build a FRESH Document, as the HTTP handler does — CreatedAt
	// deliberately left unset here to reproduce the bug shape.
	updateDoc := &model.Document{
		ID:          created.ID,
		Name:        "renamed.md",
		Content:     "updated content",
		ContentType: "text/markdown",
	}

	if err := repo.Update(ctx, updateDoc); err != nil {
		t.Fatalf("Update() returned error: %v", err)
	}

	if updateDoc.CreatedAt.IsZero() {
		t.Fatal("CreatedAt is zero after Update() — regression: created_at was lost")
	}

	// Allow for sub-second rounding differences between what Postgres
	// stored and what we read back originally; they should represent
	// the same instant.
	if !updateDoc.CreatedAt.Equal(originalCreatedAt) {
		t.Errorf("expected CreatedAt to match original %v, got %v", originalCreatedAt, updateDoc.CreatedAt)
	}
}

func TestUpdate_NotFound(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	repo := New(pool)
	ctx := context.Background()

	doc := &model.Document{
		ID:          "00000000-0000-0000-0000-000000000000",
		Name:        "nonexistent.md",
		Content:     "content",
		ContentType: "text/markdown",
	}

	err := repo.Update(ctx, doc)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestDelete(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	repo := New(pool)
	ctx := context.Background()

	doc := &model.Document{
		Name:        "delete-test.md",
		Content:     "content to delete",
		ContentType: "text/markdown",
	}
	if err := repo.Create(ctx, doc); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	if err := repo.Delete(ctx, doc.ID); err != nil {
		t.Fatalf("Delete() returned error: %v", err)
	}

	_, err := repo.GetByID(ctx, doc.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestDelete_NotFound(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()

	repo := New(pool)
	ctx := context.Background()

	err := repo.Delete(ctx, "00000000-0000-0000-0000-000000000000")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// testPool connects to TEST_DATABASE_URL, skipping the test if unset.
// The returned cleanup func closes the pool.
func testPool(t *testing.T) (*pgxpool.Pool, func()) {
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
