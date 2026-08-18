package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/vamshireddy02/mindova/services/document-service/embedding"
	"github.com/vamshireddy02/mindova/services/document-service/metrics"
	"github.com/vamshireddy02/mindova/services/document-service/model"
)

// IngestionService is the subset of service.IngestionService the worker
// depends on. Defining it locally, rather than importing the full service
// interface, keeps Worker decoupled from methods it doesn't use and makes
// it trivial to fake in tests. service.IngestionService already satisfies
// this interface structurally, so no adapter is needed when wiring up the
// real service.
type IngestionService interface {
	// ClaimNextPending atomically finds, locks, and claims the oldest
	// pending ingestion (moving it to "processing" and incrementing its
	// attempt count in one transaction), or returns (nil, nil) if
	// nothing is pending. This is what makes concurrent workers safe —
	// see repository.IngestionRepo.ClaimNextPending for the SQL.
	ClaimNextPending(ctx context.Context) (*model.Ingestion, error)
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

// defaultBatchSize caps how many ingestions Run claims and processes in
// one pass.
const defaultBatchSize = 10

// defaultPollInterval is how often RunLoop calls Run when the caller
// passes 0 for interval.
const defaultPollInterval = 5 * time.Second

// defaultMaxAttempts is how many times an ingestion is retried before
// being marked permanently failed, when the caller passes 0 for
// maxAttempts.
const defaultMaxAttempts = 3

// Worker polls for pending ingestions and drives each one through its
// full lifecycle:
//
//  1. Claim it (atomically — safe for multiple workers running at once).
//  2. Load the document.
//  3. Process it into chunks (DocumentProcessor).
//  4. Generate an embedding for each chunk (embedding.Embedder).
//  5. Persist the chunks (ChunkStore).
//  6. Mark the ingestion completed — only after everything above succeeds.
//
// A failure along the way retries the ingestion (back to "pending", so a
// later claim picks it up again) as long as attempts remain below
// maxAttempts; once exhausted, the ingestion is marked "failed"
// permanently with the last error recorded.
type Worker struct {
	ingestions  IngestionService
	documents   DocumentGetter
	processor   DocumentProcessor
	embedder    embedding.Embedder
	chunks      ChunkStore
	batchSize   int
	maxAttempts int
}

// New creates a Worker.
//
//   - batchSize controls how many ingestions Run claims and processes per
//     call; pass 0 to use the default (10).
//   - maxAttempts controls how many times a failing ingestion is retried
//     before being marked permanently failed; pass 0 to use the default
//     (3).
func New(
	ingestions IngestionService,
	documents DocumentGetter,
	processor DocumentProcessor,
	embedder embedding.Embedder,
	chunks ChunkStore,
	batchSize int,
	maxAttempts int,
) *Worker {
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxAttempts
	}

	return &Worker{
		ingestions:  ingestions,
		documents:   documents,
		processor:   processor,
		embedder:    embedder,
		chunks:      chunks,
		batchSize:   batchSize,
		maxAttempts: maxAttempts,
	}
}

// Run claims up to batchSize pending ingestions, then processes all of
// them, then returns. It does not loop or poll on its own — see RunLoop
// for continuous background processing. Run staying single-pass keeps it
// deterministic and simple to test; RunLoop is a thin wrapper around it.
//
// Claiming happens one row at a time via ClaimNextPending, but the
// entire batch is claimed BEFORE any of it is processed (see the
// implementation comment below for why that ordering matters). Each
// individual claim is still safe under concurrency: two workers (e.g.
// multiple replicas of this service) calling ClaimNextPending at the
// same time never end up claiming the same row — FOR UPDATE SKIP LOCKED
// guarantees each claim returns a different row or nil.
//
// A failure claiming work is returned to the caller. A failure
// processing an individual ingestion is not returned; it's handled by
// retrying or permanently failing that ingestion (see processOne), so
// one bad document never stops the rest of the batch.
func (w *Worker) Run(ctx context.Context) error {
	// Claim the whole batch BEFORE processing any of it. This matters:
	// if claiming and processing were interleaved (claim one, process
	// it, claim the next...), a failed ingestion that failOrRetry sets
	// back to "pending" could be re-claimed later in this SAME loop —
	// burning through all of maxAttempts in one Run() pass instead of
	// one attempt per pass as the retry design intends. Claiming
	// everything up front means a retried ingestion simply isn't in
	// this pass's batch anymore; it waits for the next Run() call.
	batch := make([]*model.Ingestion, 0, w.batchSize)
	for i := 0; i < w.batchSize; i++ {
		ing, err := w.ingestions.ClaimNextPending(ctx)
		if err != nil {
			return err
		}
		if ing == nil {
			// Nothing left pending right now.
			break
		}
		metrics.Default.RecordIngestionStarted()
		batch = append(batch, ing)
	}

	for _, ing := range batch {
		w.processOne(ctx, ing)
	}

	return nil
}

// RunLoop runs Run repeatedly, waiting interval between passes, until ctx
// is cancelled. This is what makes ingestion actually automatic: rather
// than someone manually running the worker after manually inserting a
// pending row, a single long-running RunLoop call continuously picks up
// whatever POST /documents has queued.
//
// interval of 0 uses the default (5s). onError, if non-nil, is called
// with the error from any failed Run pass — RunLoop itself never returns
// early because one pass failed (e.g. a transient database blip); it
// logs (via onError) and keeps polling. RunLoop only returns when ctx is
// cancelled, at which point it returns ctx.Err().
func (w *Worker) RunLoop(ctx context.Context, interval time.Duration, onError func(error)) error {
	if interval <= 0 {
		interval = defaultPollInterval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run immediately on start, rather than waiting for the first tick —
	// otherwise a freshly started worker sits idle for a full interval
	// before doing anything, which is a bad look right after deploy/start.
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

// processOne drives a single already-claimed ingestion through the rest
// of its lifecycle:
//
//	[claimed as processing] -> fetch document -> process document ->
//	chunk -> embed chunks -> persist chunks -> completed
//
// A failure at any step calls failOrRetry instead of failing outright —
// see failOrRetry for the retry-vs-permanently-failed decision. A
// document that processes into zero chunks is treated as a failure —
// completing an ingestion with nothing stored would be a silent no-op
// masquerading as success. Likewise, an embedder returning the wrong
// number of vectors is treated as a failure rather than persisted with
// mismatched data.
func (w *Worker) processOne(ctx context.Context, ing *model.Ingestion) {
	// Recorded via defer so every exit path — success or any of the
	// failure branches below — has its duration captured exactly once,
	// without needing a matching metrics.Default.RecordProcessingDuration
	// call next to every return statement.
	start := time.Now()
	defer func() {
		metrics.Default.RecordProcessingDuration(time.Since(start).Seconds())
	}()

	doc, err := w.documents.GetByID(ctx, ing.DocumentID)
	if err != nil {
		w.failOrRetry(ctx, ing, "failed to load document: "+err.Error())
		return
	}

	chunkTexts, err := w.processor.Process(ctx, doc)
	if err != nil {
		w.failOrRetry(ctx, ing, "failed to process document: "+err.Error())
		return
	}

	if len(chunkTexts) == 0 {
		w.failOrRetry(ctx, ing, "processing produced no chunks")
		return
	}

	embeddings, err := w.embedder.Embed(ctx, chunkTexts)
	if err != nil {
		w.failOrRetry(ctx, ing, "failed to generate embeddings: "+err.Error())
		return
	}
	if len(embeddings) != len(chunkTexts) {
		w.failOrRetry(ctx, ing, fmt.Sprintf(
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
		w.failOrRetry(ctx, ing, "failed to store chunks: "+err.Error())
		return
	}

	// Best-effort: if this final write fails, the ingestion is stuck in
	// "processing" despite its chunks being safely stored — a later
	// reconciliation step can detect and retry stuck jobs. Not
	// implemented yet.
	_ = w.ingestions.UpdateStatus(ctx, ing.ID, model.IngestionCompleted, "")
	metrics.Default.RecordCompleted()
}

// failOrRetry decides whether a failed ingestion gets another attempt or
// is marked permanently failed:
//
//	pending → processing → (fails, attempts < max) → pending → processing
//	                                                     ↓ (fails, attempts == max)
//	                                                   failed
//
// ing.Attempts was already incremented by ClaimNextPending at claim time
// — that's the single source of truth for "how many times has this been
// tried", so failOrRetry only needs to compare it against maxAttempts,
// not track its own counter.
//
// On retry, the ingestion goes back to "pending" (not straight back to
// "processing") so the normal claim path picks it up again on a later
// pass — this also means a retried ingestion naturally goes to the back
// of the queue behind other pending work, rather than being retried
// immediately in a tight loop.
func (w *Worker) failOrRetry(ctx context.Context, ing *model.Ingestion, message string) {
	if ing.Attempts < w.maxAttempts {
		_ = w.ingestions.UpdateStatus(ctx, ing.ID, model.IngestionPending, message)
		metrics.Default.RecordRetried()
		return
	}
	_ = w.ingestions.UpdateStatus(ctx, ing.ID, model.IngestionFailed, message)
	metrics.Default.RecordFailed()
}
