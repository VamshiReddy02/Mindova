package service

import (
	"context"

	"github.com/vamshireddy02/mindova/services/document-service/model"
)

type DocumentService interface {
	Create(ctx context.Context, doc *model.Document) error
	GetByID(ctx context.Context, id string) (*model.Document, error)
	List(ctx context.Context, limit int) ([]*model.Document, error)
	Update(ctx context.Context, doc *model.Document) error
	Delete(ctx context.Context, id string) error
}
