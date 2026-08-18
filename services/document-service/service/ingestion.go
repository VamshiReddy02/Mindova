package service

import (
	"context"

	"github.com/vamshireddy02/mindova/services/document-service/model"
	"github.com/vamshireddy02/mindova/services/document-service/repository"
)

type IngestionService interface {
	Create(ctx context.Context, ingestion *model.Ingestion) error
	GetByID(ctx context.Context, id string) (*model.Ingestion, error)
	GetByDocumentID(ctx context.Context, documentID string) ([]*model.Ingestion, error)
	ListPending(ctx context.Context, limit int) ([]*model.Ingestion, error)
	ClaimNextPending(ctx context.Context) (*model.Ingestion, error)
	UpdateStatus(
		ctx context.Context,
		id string,
		status model.IngestionStatus,
		errMsg string,
	) error
}

type ingestionService struct {
	repo repository.IngestionRepository
}

func NewIngestionService(repo repository.IngestionRepository) IngestionService {
	return &ingestionService{
		repo: repo,
	}
}

func (s *ingestionService) Create(
	ctx context.Context,
	ingestion *model.Ingestion,
) error {
	if ingestion.Status == "" {
		ingestion.Status = model.IngestionPending
	}

	return s.repo.Create(ctx, ingestion)
}

func (s *ingestionService) GetByID(
	ctx context.Context,
	id string,
) (*model.Ingestion, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *ingestionService) GetByDocumentID(
	ctx context.Context,
	documentID string,
) ([]*model.Ingestion, error) {
	return s.repo.GetByDocumentID(ctx, documentID)
}

func (s *ingestionService) ListPending(
	ctx context.Context,
	limit int,
) ([]*model.Ingestion, error) {
	return s.repo.ListPending(ctx, limit)
}

func (s *ingestionService) ClaimNextPending(
	ctx context.Context,
) (*model.Ingestion, error) {
	return s.repo.ClaimNextPending(ctx)
}

func (s *ingestionService) UpdateStatus(
	ctx context.Context,
	id string,
	status model.IngestionStatus,
	errMsg string,
) error {
	return s.repo.UpdateStatus(ctx, id, status, errMsg)
}
