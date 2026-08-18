package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vamshireddy02/mindova/services/document-service/model"
)

type mockIngestionRepository struct {
	createFn           func(context.Context, *model.Ingestion) error
	getByIDFn          func(context.Context, string) (*model.Ingestion, error)
	getByDocumentIDFn  func(context.Context, string) ([]*model.Ingestion, error)
	listPendingFn      func(context.Context, int) ([]*model.Ingestion, error)
	claimNextPendingFn func(context.Context) (*model.Ingestion, error)
	updateStatusFn     func(context.Context, string, model.IngestionStatus, string) error
}

func (m *mockIngestionRepository) Create(
	ctx context.Context,
	ingestion *model.Ingestion,
) error {
	return m.createFn(ctx, ingestion)
}

func (m *mockIngestionRepository) GetByID(
	ctx context.Context,
	id string,
) (*model.Ingestion, error) {
	return m.getByIDFn(ctx, id)
}

func (m *mockIngestionRepository) GetByDocumentID(
	ctx context.Context,
	documentID string,
) ([]*model.Ingestion, error) {
	return m.getByDocumentIDFn(ctx, documentID)
}

func (m *mockIngestionRepository) ListPending(
	ctx context.Context,
	limit int,
) ([]*model.Ingestion, error) {
	return m.listPendingFn(ctx, limit)
}

func (m *mockIngestionRepository) ClaimNextPending(
	ctx context.Context,
) (*model.Ingestion, error) {
	return m.claimNextPendingFn(ctx)
}

func (m *mockIngestionRepository) UpdateStatus(
	ctx context.Context,
	id string,
	status model.IngestionStatus,
	errMsg string,
) error {
	return m.updateStatusFn(ctx, id, status, errMsg)
}

func TestIngestionService_Create_SetsPendingStatus(t *testing.T) {
	var created *model.Ingestion

	repo := &mockIngestionRepository{
		createFn: func(
			ctx context.Context,
			ingestion *model.Ingestion,
		) error {
			created = ingestion
			return nil
		},
	}

	svc := NewIngestionService(repo)

	ingestion := &model.Ingestion{
		DocumentID: "document-123",
	}

	err := svc.Create(context.Background(), ingestion)
	if err != nil {
		t.Fatalf("Create() returned error: %v", err)
	}

	if created == nil {
		t.Fatal("repository Create() was not called")
	}

	if created.Status != model.IngestionPending {
		t.Fatalf(
			"expected status %q, got %q",
			model.IngestionPending,
			created.Status,
		)
	}
}

func TestIngestionService_Create_PreservesStatus(t *testing.T) {
	repo := &mockIngestionRepository{
		createFn: func(
			ctx context.Context,
			ingestion *model.Ingestion,
		) error {
			return nil
		},
	}

	svc := NewIngestionService(repo)

	ingestion := &model.Ingestion{
		DocumentID: "document-123",
		Status:     model.IngestionProcessing,
	}

	err := svc.Create(context.Background(), ingestion)
	if err != nil {
		t.Fatalf("Create() returned error: %v", err)
	}

	if ingestion.Status != model.IngestionProcessing {
		t.Fatalf(
			"expected status %q, got %q",
			model.IngestionProcessing,
			ingestion.Status,
		)
	}
}

func TestIngestionService_Create_PropagatesError(t *testing.T) {
	expectedErr := errors.New("repository error")

	repo := &mockIngestionRepository{
		createFn: func(
			ctx context.Context,
			ingestion *model.Ingestion,
		) error {
			return expectedErr
		},
	}

	svc := NewIngestionService(repo)

	ingestion := &model.Ingestion{
		DocumentID: "document-123",
	}

	err := svc.Create(context.Background(), ingestion)

	if !errors.Is(err, expectedErr) {
		t.Fatalf(
			"expected error %v, got %v",
			expectedErr,
			err,
		)
	}
}

func TestIngestionService_GetByID(t *testing.T) {
	expected := &model.Ingestion{
		ID:         "ingestion-123",
		DocumentID: "document-123",
		Status:     model.IngestionPending,
		CreatedAt:  time.Now(),
	}

	repo := &mockIngestionRepository{
		getByIDFn: func(
			ctx context.Context,
			id string,
		) (*model.Ingestion, error) {
			if id != "ingestion-123" {
				t.Fatalf("unexpected id: %s", id)
			}

			return expected, nil
		},
	}

	svc := NewIngestionService(repo)

	got, err := svc.GetByID(
		context.Background(),
		"ingestion-123",
	)

	if err != nil {
		t.Fatalf("GetByID() returned error: %v", err)
	}

	if got != expected {
		t.Fatal("GetByID() returned unexpected ingestion")
	}
}

func TestIngestionService_GetByID_PropagatesError(t *testing.T) {
	expectedErr := errors.New("not found")

	repo := &mockIngestionRepository{
		getByIDFn: func(
			ctx context.Context,
			id string,
		) (*model.Ingestion, error) {
			return nil, expectedErr
		},
	}

	svc := NewIngestionService(repo)

	_, err := svc.GetByID(
		context.Background(),
		"ingestion-123",
	)

	if !errors.Is(err, expectedErr) {
		t.Fatalf(
			"expected error %v, got %v",
			expectedErr,
			err,
		)
	}
}

func TestIngestionService_GetByDocumentID(t *testing.T) {
	expected := []*model.Ingestion{
		{
			ID:         "ingestion-1",
			DocumentID: "document-123",
			Status:     model.IngestionCompleted,
		},
		{
			ID:         "ingestion-2",
			DocumentID: "document-123",
			Status:     model.IngestionFailed,
		},
	}

	repo := &mockIngestionRepository{
		getByDocumentIDFn: func(
			ctx context.Context,
			documentID string,
		) ([]*model.Ingestion, error) {
			if documentID != "document-123" {
				t.Fatalf("unexpected document ID: %s", documentID)
			}

			return expected, nil
		},
	}

	svc := NewIngestionService(repo)

	got, err := svc.GetByDocumentID(
		context.Background(),
		"document-123",
	)

	if err != nil {
		t.Fatalf(
			"GetByDocumentID() returned error: %v",
			err,
		)
	}

	if len(got) != 2 {
		t.Fatalf(
			"expected 2 ingestions, got %d",
			len(got),
		)
	}
}

func TestIngestionService_GetByDocumentID_PropagatesError(t *testing.T) {
	expectedErr := errors.New("repository error")

	repo := &mockIngestionRepository{
		getByDocumentIDFn: func(
			ctx context.Context,
			documentID string,
		) ([]*model.Ingestion, error) {
			return nil, expectedErr
		},
	}

	svc := NewIngestionService(repo)

	_, err := svc.GetByDocumentID(
		context.Background(),
		"document-123",
	)

	if !errors.Is(err, expectedErr) {
		t.Fatalf(
			"expected error %v, got %v",
			expectedErr,
			err,
		)
	}
}

func TestIngestionService_ListPending(t *testing.T) {
	expected := []*model.Ingestion{
		{ID: "ingestion-1", Status: model.IngestionPending},
		{ID: "ingestion-2", Status: model.IngestionPending},
	}

	var gotLimit int

	repo := &mockIngestionRepository{
		listPendingFn: func(
			ctx context.Context,
			limit int,
		) ([]*model.Ingestion, error) {
			gotLimit = limit
			return expected, nil
		},
	}

	svc := NewIngestionService(repo)

	got, err := svc.ListPending(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListPending() returned error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 pending ingestions, got %d", len(got))
	}
	if gotLimit != 10 {
		t.Fatalf("expected repository called with limit=10, got %d", gotLimit)
	}
}

func TestIngestionService_ListPending_PropagatesError(t *testing.T) {
	expectedErr := errors.New("repository error")

	repo := &mockIngestionRepository{
		listPendingFn: func(
			ctx context.Context,
			limit int,
		) ([]*model.Ingestion, error) {
			return nil, expectedErr
		},
	}

	svc := NewIngestionService(repo)

	_, err := svc.ListPending(context.Background(), 10)
	if !errors.Is(err, expectedErr) {
		t.Fatalf(
			"expected error %v, got %v",
			expectedErr,
			err,
		)
	}
}

func TestIngestionService_ClaimNextPending(t *testing.T) {
	expected := &model.Ingestion{
		ID:       "ingestion-1",
		Status:   model.IngestionProcessing,
		Attempts: 1,
	}

	repo := &mockIngestionRepository{
		claimNextPendingFn: func(ctx context.Context) (*model.Ingestion, error) {
			return expected, nil
		},
	}

	svc := NewIngestionService(repo)

	got, err := svc.ClaimNextPending(context.Background())
	if err != nil {
		t.Fatalf("ClaimNextPending() returned error: %v", err)
	}
	if got != expected {
		t.Fatal("ClaimNextPending() returned unexpected ingestion")
	}
}

func TestIngestionService_ClaimNextPending_NoneAvailable(t *testing.T) {
	repo := &mockIngestionRepository{
		claimNextPendingFn: func(ctx context.Context) (*model.Ingestion, error) {
			return nil, nil
		},
	}

	svc := NewIngestionService(repo)

	got, err := svc.ClaimNextPending(context.Background())
	if err != nil {
		t.Fatalf("ClaimNextPending() returned error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil when nothing is pending, got %+v", got)
	}
}

func TestIngestionService_ClaimNextPending_PropagatesError(t *testing.T) {
	expectedErr := errors.New("repository error")

	repo := &mockIngestionRepository{
		claimNextPendingFn: func(ctx context.Context) (*model.Ingestion, error) {
			return nil, expectedErr
		},
	}

	svc := NewIngestionService(repo)

	_, err := svc.ClaimNextPending(context.Background())
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
}

func TestIngestionService_UpdateStatus(t *testing.T) {
	var (
		gotID     string
		gotStatus model.IngestionStatus
		gotErrMsg string
	)

	repo := &mockIngestionRepository{
		updateStatusFn: func(
			ctx context.Context,
			id string,
			status model.IngestionStatus,
			errMsg string,
		) error {
			gotID = id
			gotStatus = status
			gotErrMsg = errMsg

			return nil
		},
	}

	svc := NewIngestionService(repo)

	err := svc.UpdateStatus(
		context.Background(),
		"ingestion-123",
		model.IngestionCompleted,
		"",
	)

	if err != nil {
		t.Fatalf(
			"UpdateStatus() returned error: %v",
			err,
		)
	}

	if gotID != "ingestion-123" {
		t.Fatalf("expected ID ingestion-123, got %s", gotID)
	}

	if gotStatus != model.IngestionCompleted {
		t.Fatalf(
			"expected status %q, got %q",
			model.IngestionCompleted,
			gotStatus,
		)
	}

	if gotErrMsg != "" {
		t.Fatalf(
			"expected empty error message, got %q",
			gotErrMsg,
		)
	}
}

func TestIngestionService_UpdateStatus_PropagatesError(t *testing.T) {
	expectedErr := errors.New("repository error")

	repo := &mockIngestionRepository{
		updateStatusFn: func(
			ctx context.Context,
			id string,
			status model.IngestionStatus,
			errMsg string,
		) error {
			return expectedErr
		},
	}

	svc := NewIngestionService(repo)

	err := svc.UpdateStatus(
		context.Background(),
		"ingestion-123",
		model.IngestionFailed,
		"processing failed",
	)

	if !errors.Is(err, expectedErr) {
		t.Fatalf(
			"expected error %v, got %v",
			expectedErr,
			err,
		)
	}
}
