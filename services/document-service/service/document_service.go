package service

import (
	"context"

	"github.com/vamshireddy02/mindova/services/document-service/model"
	"github.com/vamshireddy02/mindova/services/document-service/repository"
)

type Service struct {
	repo repository.DocumentRepository
}

func New(repo repository.DocumentRepository) *Service {
	return &Service{
		repo: repo,
	}
}

func (s *Service) Create(ctx context.Context, doc *model.Document) error {
	return s.repo.Create(ctx, doc)
}

func (s *Service) GetByID(ctx context.Context, id string) (*model.Document, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]*model.Document, error) {
	return s.repo.List(ctx)
}

func (s *Service) Update(ctx context.Context, doc *model.Document) error {
	return s.repo.Update(ctx, doc)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

var _ DocumentService = (*Service)(nil)
