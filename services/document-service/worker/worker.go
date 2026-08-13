package worker

import (
	"context"

	"github.com/vamshireddy02/mindova/services/document-service/model"
)

// IngestionService is the subset of service.IngestionService the worker
// depends on. Defining it locally, rather than importing the full service
// interface, keeps Worker decoupled from methods it doesn't use and makes
// it trivial to fake in tests. service.IngestionService already satisfies
// this interface structurally, so no adapter is needed when wiring up the
// real service.
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

// Processor performs the actual document processing work — chunking,
// generating embeddings, writing to a vector database, and so on. This is
// deliberately a seam: no real implementation exists yet. A caller can
// supply a no-op or stub Processor now, and swap in the real pipeline
// later without touching Worker.
type Processor interface {
	Process(ctx context.Context, doc *model.Document) error
}

// defaultBatchSize caps how many pending ingestions Run pulls in one pass.
const defaultBatchSize = 10

// Worker polls for pending ingestions and drives each one through its
// pending -> processing -> completed/failed lifecycle.
type Worker struct {
	ingestions IngestionService
	documents  DocumentGetter
	processor  Processor
	batchSize  int
}

// New creates a Worker. batchSize controls how many pending ingestions
// Run processes per call; pass 0 to use the default (10).
func New(ingestions IngestionService, documents DocumentGetter, processor Processor, batchSize int) *Worker {
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}

	return &Worker{
		ingestions: ingestions,
		documents:  documents,
		processor:  processor,
		batchSize:  batchSize,
	}
}

// Run fetches one batch of pending ingestions and processes each in turn,
// then returns. It does not loop or poll on its own — callers that want
// continuous background processing wrap Run in their own ticker/loop. This
// keeps Run deterministic and simple to test.
//
// A failure fetching the pending batch is returned to the caller. A
// failure processing an individual ingestion is not returned; it's
// recorded as that ingestion's failed status instead, so one bad document
// never stops the rest of the batch.
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

// processOne drives a single ingestion through pending -> processing ->
// completed/failed.
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

	if err := w.processor.Process(ctx, doc); err != nil {
		w.fail(ctx, ing.ID, err.Error())
		return
	}

	// Best-effort: if this final write fails, the ingestion is stuck in
	// "processing" rather than silently lost — a later reconciliation
	// step can detect and retry stuck jobs. Not implemented yet.
	_ = w.ingestions.UpdateStatus(ctx, ing.ID, model.IngestionCompleted, "")
}

// fail marks an ingestion as failed with the given message. Errors from
// this call itself are swallowed for the same reason as in processOne:
// there's no safe further action to take within a single Run pass.
func (w *Worker) fail(ctx context.Context, id string, message string) {
	_ = w.ingestions.UpdateStatus(ctx, id, model.IngestionFailed, message)
}
