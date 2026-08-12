package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vamshireddy02/mindova/services/document-service/model"
	"github.com/vamshireddy02/mindova/services/document-service/repository"
)

// mockRepository is an in-memory stand-in for repository.DocumentRepository.
// Like handler's stubService, each test wires only the function field(s)
// it needs; an unset field returns errUnimplemented so an unexpected call
// fails loudly instead of nil-panicking or silently succeeding.
type mockRepository struct {
	createFn  func(ctx context.Context, doc *model.Document) error
	getByIDFn func(ctx context.Context, id string) (*model.Document, error)
	listFn    func(ctx context.Context, limit int) ([]*model.Document, error)
	updateFn  func(ctx context.Context, doc *model.Document) error
	deleteFn  func(ctx context.Context, id string) error
}

var errUnimplemented = errors.New("mockRepository: method not implemented for this test")

func (m *mockRepository) Create(ctx context.Context, doc *model.Document) error {
	if m.createFn == nil {
		return errUnimplemented
	}
	return m.createFn(ctx, doc)
}

func (m *mockRepository) GetByID(ctx context.Context, id string) (*model.Document, error) {
	if m.getByIDFn == nil {
		return nil, errUnimplemented
	}
	return m.getByIDFn(ctx, id)
}

func (m *mockRepository) List(ctx context.Context, limit int) ([]*model.Document, error) {
	if m.listFn == nil {
		return nil, errUnimplemented
	}
	return m.listFn(ctx, limit)
}

func (m *mockRepository) Update(ctx context.Context, doc *model.Document) error {
	if m.updateFn == nil {
		return errUnimplemented
	}
	return m.updateFn(ctx, doc)
}

func (m *mockRepository) Delete(ctx context.Context, id string) error {
	if m.deleteFn == nil {
		return errUnimplemented
	}
	return m.deleteFn(ctx, id)
}

// Compile-time check that mockRepository satisfies the interface Service depends on.
var _ repository.DocumentRepository = (*mockRepository)(nil)

// --- Create ---------------------------------------------------------------

func TestService_Create_CallsRepository(t *testing.T) {
	called := false

	repo := &mockRepository{
		createFn: func(ctx context.Context, doc *model.Document) error {
			called = true
			if doc.Name != "architecture.md" {
				t.Errorf("expected name architecture.md, got %s", doc.Name)
			}
			return nil
		},
	}
	svc := New(repo)

	doc := &model.Document{Name: "architecture.md", Content: "...", ContentType: "text/markdown"}
	if err := svc.Create(context.Background(), doc); err != nil {
		t.Fatalf("Create() returned error: %v", err)
	}

	if !called {
		t.Error("expected repository.Create to be called")
	}
}

func TestService_Create_PropagatesError(t *testing.T) {
	wantErr := errors.New("insert failed")

	repo := &mockRepository{
		createFn: func(ctx context.Context, doc *model.Document) error {
			return wantErr
		},
	}
	svc := New(repo)

	err := svc.Create(context.Background(), &model.Document{})
	if !errors.Is(err, wantErr) {
		t.Errorf("expected error %v, got %v", wantErr, err)
	}
}

// --- GetByID ----------------------------------------------------------------

func TestService_GetByID_ReturnsDocument(t *testing.T) {
	want := &model.Document{
		ID:        "doc-123",
		Name:      "architecture.md",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	repo := &mockRepository{
		getByIDFn: func(ctx context.Context, id string) (*model.Document, error) {
			if id != "doc-123" {
				t.Fatalf("expected id doc-123, got %s", id)
			}
			return want, nil
		},
	}
	svc := New(repo)

	got, err := svc.GetByID(context.Background(), "doc-123")
	if err != nil {
		t.Fatalf("GetByID() returned error: %v", err)
	}
	if got != want {
		t.Errorf("expected returned document to be the repository's document")
	}
}

func TestService_GetByID_PropagatesNotFound(t *testing.T) {
	repo := &mockRepository{
		getByIDFn: func(ctx context.Context, id string) (*model.Document, error) {
			return nil, repository.ErrNotFound
		},
	}
	svc := New(repo)

	_, err := svc.GetByID(context.Background(), "missing-id")
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- List ---------------------------------------------------------------

func TestService_List_ReturnsDocuments(t *testing.T) {
	want := []*model.Document{
		{ID: "doc-1", Name: "a.md"},
		{ID: "doc-2", Name: "b.md"},
	}

	repo := &mockRepository{
		listFn: func(ctx context.Context, limit int) ([]*model.Document, error) {
			return want, nil
		},
	}
	svc := New(repo)

	got, err := svc.List(context.Background(), 20)
	if err != nil {
		t.Fatalf("List() returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 documents, got %d", len(got))
	}
}

func TestService_List_PassesLimitThrough(t *testing.T) {
	var receivedLimit int

	repo := &mockRepository{
		listFn: func(ctx context.Context, limit int) ([]*model.Document, error) {
			receivedLimit = limit
			return nil, nil
		},
	}
	svc := New(repo)

	if _, err := svc.List(context.Background(), 5); err != nil {
		t.Fatalf("List() returned error: %v", err)
	}

	if receivedLimit != 5 {
		t.Errorf("expected repository.List called with limit=5, got %d", receivedLimit)
	}
}

func TestService_List_PropagatesError(t *testing.T) {
	wantErr := errors.New("query failed")

	repo := &mockRepository{
		listFn: func(ctx context.Context, limit int) ([]*model.Document, error) {
			return nil, wantErr
		},
	}
	svc := New(repo)

	_, err := svc.List(context.Background(), 20)
	if !errors.Is(err, wantErr) {
		t.Errorf("expected error %v, got %v", wantErr, err)
	}
}

// --- Update ---------------------------------------------------------------

func TestService_Update_CallsRepository(t *testing.T) {
	called := false

	repo := &mockRepository{
		updateFn: func(ctx context.Context, doc *model.Document) error {
			called = true
			if doc.ID != "doc-123" {
				t.Errorf("expected id doc-123, got %s", doc.ID)
			}
			return nil
		},
	}
	svc := New(repo)

	doc := &model.Document{ID: "doc-123", Name: "renamed.md"}
	if err := svc.Update(context.Background(), doc); err != nil {
		t.Fatalf("Update() returned error: %v", err)
	}

	if !called {
		t.Error("expected repository.Update to be called")
	}
}

func TestService_Update_PropagatesNotFound(t *testing.T) {
	repo := &mockRepository{
		updateFn: func(ctx context.Context, doc *model.Document) error {
			return repository.ErrNotFound
		},
	}
	svc := New(repo)

	err := svc.Update(context.Background(), &model.Document{ID: "missing-id"})
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- Delete ---------------------------------------------------------------

func TestService_Delete_CallsRepository(t *testing.T) {
	called := false

	repo := &mockRepository{
		deleteFn: func(ctx context.Context, id string) error {
			called = true
			if id != "doc-123" {
				t.Errorf("expected id doc-123, got %s", id)
			}
			return nil
		},
	}
	svc := New(repo)

	if err := svc.Delete(context.Background(), "doc-123"); err != nil {
		t.Fatalf("Delete() returned error: %v", err)
	}

	if !called {
		t.Error("expected repository.Delete to be called")
	}
}

func TestService_Delete_PropagatesNotFound(t *testing.T) {
	repo := &mockRepository{
		deleteFn: func(ctx context.Context, id string) error {
			return repository.ErrNotFound
		},
	}
	svc := New(repo)

	err := svc.Delete(context.Background(), "missing-id")
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
