package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/vamshireddy02/mindova/services/document-service/embedding"
	"github.com/vamshireddy02/mindova/services/document-service/model"
)

type IngestionService interface {
	ListPending(ctx context.Context, limit int) ([]*model.Ingestion, error)
	UpdateStatus(ctx context.Context, id string, status model.IngestionStatus, errMsg string) error
}

// DocumentGetter is the subset of service.DocumentService the worker needs
// to load the document being ingested. service.DocumentService already
// satisfies this interface structurally.
type DocumentGetter interface {
	GetByID(ctx context.Context, id string) (*model.Document, error)
}

// ChunkStore is the subset of repository.ChunkRepository the worker needs
// to persist chunks produced by a DocumentProcessor. repository.ChunkRepo
// already satisfies this interface structurally.
type ChunkStore interface {
	CreateBatch(ctx context.Context, chunks []*model.DocumentChunk) error
}

// defaultBatchSize caps how many pending ingestions Run pulls in one pass.
const defaultBatchSize = 10

// defaultPollInterval is how often RunLoop calls Run when the caller
// passes 0 for interval.
const defaultPollInterval = 5 * time.Second

// Worker polls for pending ingestions and drives each one through its
// full lifecycle:
//
//  1. Load the document.
//  2. Process it into chunks (DocumentProcessor).
//  3. Generate an embedding for each chunk (embedding.Embedder).
//  4. Persist the chunks (ChunkStore).
//  5. Mark the ingestion completed — only after everything above succeeds.
//
// Any failure along the way marks the ingestion failed with the error
// recorded, rather than leaving it stuck in processing.
type Worker struct {
	ingestions IngestionService
	documents  DocumentGetter
	processor  DocumentProcessor
	embedder   embedding.Embedder
	chunks     ChunkStore
	batchSize  int
}

// New creates a Worker. batchSize controls how many pending ingestions
// Run processes per call; pass 0 to use the default (10).
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

func (w *Worker) RunLoop(ctx context.Context, interval time.Duration, onError func(error)) error {
	if interval <= 0 {
		interval = defaultPollInterval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	if err := w.Run(ctx); err != nil && onError != nil {
		onError(err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := w.Run(ctx); err != nil && onError != nil {
				onError(err)
			}
		}
	}
}

// mismatched data.
func (w *Worker) processOne(ctx context.Context, ing *model.Ingestion) {
	if err := w.ingestions.UpdateStatus(ctx, ing.ID, model.IngestionProcessing, ""); err != nil {
		// Couldn't even mark it as processing — leave it as-is rather than
		// risk masking the real problem with a second failing write.
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
