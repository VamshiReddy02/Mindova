package worker

import (
	"context"

	"github.com/vamshireddy02/mindova/services/document-service/model"
)

type IngestionService interface {
	ListPending(ctx context.Context, limit int) ([]*model.Ingestion, error)
	UpdateStatus(ctx context.Context, id string, status model.IngestionStatus, errMsg string) error
}

type DocumentGetter interface {
	GetByID(ctx context.Context, id string) (*model.Document, error)
}

type ChunkStore interface {
	CreateBatch(ctx context.Context, chunks []*model.DocumentChunk) error
}

const defaultBatchSize = 10

type Worker struct {
	ingestions IngestionService
	documents  DocumentGetter
	processor  DocumentProcessor
	chunks     ChunkStore
	batchSize  int
}

// New creates a Worker. batchSize controls how many pending ingestions
// Run processes per call; pass 0 to use the default (10).
func New(ingestions IngestionService, documents DocumentGetter, processor DocumentProcessor, chunks ChunkStore, batchSize int) *Worker {
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}

	return &Worker{
		ingestions: ingestions,
		documents:  documents,
		processor:  processor,
		chunks:     chunks,
		batchSize:  batchSize,
	}
}

func (w *Worker) Run(ctx context.Context) error {
	pending, err := w.ingestions.ListPending(ctx, w.batchSize)
	if err != nil {
		return err
	}

	for _, ing := range pending {
		w.processOne(ctx, ing)
	}

	return nil
}

func (w *Worker) processOne(ctx context.Context, ing *model.Ingestion) {
	if err := w.ingestions.UpdateStatus(ctx, ing.ID, model.IngestionProcessing, ""); err != nil {
		return
	}

	doc, err := w.documents.GetByID(ctx, ing.DocumentID)
	if err != nil {
		w.fail(ctx, ing.ID, "failed to load document: "+err.Error())
		return
	}

	chunkTexts, err := w.processor.Process(ctx, doc)
	if err != nil {
		w.fail(ctx, ing.ID, "failed to process document: "+err.Error())
		return
	}

	if len(chunkTexts) == 0 {
		w.fail(ctx, ing.ID, "processing produced no chunks")
		return
	}

	chunks := make([]*model.DocumentChunk, len(chunkTexts))
	for i, text := range chunkTexts {
		chunks[i] = &model.DocumentChunk{
			DocumentID: doc.ID,
			ChunkIndex: i,
			Content:    text,
		}
	}

	if err := w.chunks.CreateBatch(ctx, chunks); err != nil {
		w.fail(ctx, ing.ID, "failed to store chunks: "+err.Error())
		return
	}

	_ = w.ingestions.UpdateStatus(ctx, ing.ID, model.IngestionCompleted, "")
}

func (w *Worker) fail(ctx context.Context, id string, message string) {
	_ = w.ingestions.UpdateStatus(ctx, id, model.IngestionFailed, message)
}
