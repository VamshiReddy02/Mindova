package handler

import (
	"context"
	"errors"
	"io"

	"github.com/vamshireddy02/mindova/packages/kernel/config"
	"github.com/vamshireddy02/mindova/packages/kernel/logger"
	"github.com/vamshireddy02/mindova/services/document-service/model"
	"github.com/vamshireddy02/mindova/services/document-service/service"
)

// stubService is a minimal in-memory DocumentService for handler tests.
// Each test sets only the function field(s) it needs; calling an unset
// method fails loudly via errUnimplemented rather than nil-panicking,
// which makes accidental cross-calls between handlers obvious in test
// output.
type stubService struct {
	createFn  func(ctx context.Context, doc *model.Document) error
	getByIDFn func(ctx context.Context, id string) (*model.Document, error)
	listFn    func(ctx context.Context, limit int) ([]*model.Document, error)
	updateFn  func(ctx context.Context, doc *model.Document) error
	deleteFn  func(ctx context.Context, id string) error
}

var errUnimplemented = errors.New("stubService: method not implemented for this test")

func (s *stubService) Create(ctx context.Context, doc *model.Document) error {
	if s.createFn == nil {
		return errUnimplemented
	}
	return s.createFn(ctx, doc)
}

func (s *stubService) GetByID(ctx context.Context, id string) (*model.Document, error) {
	if s.getByIDFn == nil {
		return nil, errUnimplemented
	}
	return s.getByIDFn(ctx, id)
}

func (s *stubService) List(ctx context.Context, limit int) ([]*model.Document, error) {
	if s.listFn == nil {
		return nil, errUnimplemented
	}
	return s.listFn(ctx, limit)
}

func (s *stubService) Update(ctx context.Context, doc *model.Document) error {
	if s.updateFn == nil {
		return errUnimplemented
	}
	return s.updateFn(ctx, doc)
}

func (s *stubService) Delete(ctx context.Context, id string) error {
	if s.deleteFn == nil {
		return errUnimplemented
	}
	return s.deleteFn(ctx, id)
}

// stubRetrievalService is a minimal in-memory RetrievalService for handler
// tests, following the same unset-field-fails-loudly convention as
// stubService.
type stubRetrievalService struct {
	searchFn func(ctx context.Context, query string, limit int) ([]*model.DocumentChunk, error)
}

func (s *stubRetrievalService) Search(ctx context.Context, query string, limit int) ([]*model.DocumentChunk, error) {
	if s.searchFn == nil {
		return nil, errUnimplemented
	}
	return s.searchFn(ctx, query, limit)
}

// stubRAGService is a minimal in-memory RAGService for handler tests,
// following the same unset-field-fails-loudly convention as the other
// stubs in this file.
type stubRAGService struct {
	askFn func(ctx context.Context, question string, limit int) (*service.RAGResponse, error)
}

func (s *stubRAGService) Ask(ctx context.Context, question string, limit int) (*service.RAGResponse, error) {
	if s.askFn == nil {
		return nil, errUnimplemented
	}
	return s.askFn(ctx, question, limit)
}

// stubIngestionService is a minimal in-memory IngestionService for
// handler tests, following the same unset-field-fails-loudly convention
// as the other stubs in this file. Only createFn is exercised by
// handler tests today (Create's best-effort ingestion enqueue); the rest
// exist purely to satisfy service.IngestionService.
type stubIngestionService struct {
	createFn           func(ctx context.Context, ingestion *model.Ingestion) error
	getByIDFn          func(ctx context.Context, id string) (*model.Ingestion, error)
	getByDocumentIDFn  func(ctx context.Context, documentID string) ([]*model.Ingestion, error)
	listPendingFn      func(ctx context.Context, limit int) ([]*model.Ingestion, error)
	claimNextPendingFn func(ctx context.Context) (*model.Ingestion, error)
	updateStatusFn     func(ctx context.Context, id string, status model.IngestionStatus, errMsg string) error
}

func (s *stubIngestionService) Create(ctx context.Context, ingestion *model.Ingestion) error {
	if s.createFn == nil {
		return errUnimplemented
	}
	return s.createFn(ctx, ingestion)
}

func (s *stubIngestionService) GetByID(ctx context.Context, id string) (*model.Ingestion, error) {
	if s.getByIDFn == nil {
		return nil, errUnimplemented
	}
	return s.getByIDFn(ctx, id)
}

func (s *stubIngestionService) GetByDocumentID(ctx context.Context, documentID string) ([]*model.Ingestion, error) {
	if s.getByDocumentIDFn == nil {
		return nil, errUnimplemented
	}
	return s.getByDocumentIDFn(ctx, documentID)
}

func (s *stubIngestionService) ListPending(ctx context.Context, limit int) ([]*model.Ingestion, error) {
	if s.listPendingFn == nil {
		return nil, errUnimplemented
	}
	return s.listPendingFn(ctx, limit)
}

func (s *stubIngestionService) ClaimNextPending(ctx context.Context) (*model.Ingestion, error) {
	if s.claimNextPendingFn == nil {
		return nil, errUnimplemented
	}
	return s.claimNextPendingFn(ctx)
}

func (s *stubIngestionService) UpdateStatus(ctx context.Context, id string, status model.IngestionStatus, errMsg string) error {
	if s.updateStatusFn == nil {
		return errUnimplemented
	}
	return s.updateStatusFn(ctx, id, status, errMsg)
}

// testLogger returns a *logger.Logger that discards everything it
// writes — handler tests need a real, non-nil logger (Create calls
// h.log.Error on a best-effort ingestion enqueue failure), but test
// output shouldn't be cluttered with expected-failure log lines.
func testLogger() *logger.Logger {
	return logger.NewWithWriter(config.AppConfig{
		Environment: "development",
		LogLevel:    "error",
	}, io.Discard)
}
