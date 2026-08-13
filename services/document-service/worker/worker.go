package worker

import (
	"context"
	"fmt"

	"github.com/vamshireddy02/mindova/services/document-service/embedding"
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
	embedder   embedding.Embedder
	chunks     ChunkStore
	batchSize  int
}

func New(
	ingestions IngestionService,
	documents DocumentGetter,
	processor DocumentProcessor,
	embedder embedding.Embedder,
	chunks ChunkStore,
	batchSize int,
) *Worker {
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}

	return &Worker{
		ingestions: ingestions,
		documents:  documents,
		processor:  processor,
		embedder:   embedder,
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

	embeddings, err := w.embedder.Embed(ctx, chunkTexts)
	if err != nil {
		w.fail(ctx, ing.ID, "failed to generate embeddings: "+err.Error())
		return
	}
	if len(embeddings) != len(chunkTexts) {
		w.fail(ctx, ing.ID, fmt.Sprintf(
			"embedding count mismatch: expected %d, got %d",
			len(chunkTexts), len(embeddings),
		))
		return
	}

	chunks := make([]*model.DocumentChunk, len(chunkTexts))
	for i, text := range chunkTexts {
		chunks[i] = &model.DocumentChunk{
			DocumentID: doc.ID,
			ChunkIndex: i,
			Content:    text,
			Embedding:  embeddings[i],
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
