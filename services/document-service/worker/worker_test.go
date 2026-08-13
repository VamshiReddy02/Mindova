package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/vamshireddy02/mindova/services/document-service/model"
)

// statusCall records a single UpdateStatus invocation, so tests can assert
// both which calls happened and the order they happened in.
type statusCall struct {
	id     string
	status model.IngestionStatus
	errMsg string
}

// fakeIngestionService is an in-memory stand-in for worker.IngestionService.
type fakeIngestionService struct {
	pending []*model.Ingestion
	listErr error

	calls []statusCall
}

func (f *fakeIngestionService) ListPending(ctx context.Context, limit int) ([]*model.Ingestion, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	if limit < len(f.pending) {
		return f.pending[:limit], nil
	}
	return f.pending, nil
}

func (f *fakeIngestionService) UpdateStatus(ctx context.Context, id string, status model.IngestionStatus, errMsg string) error {
	f.calls = append(f.calls, statusCall{id: id, status: status, errMsg: errMsg})
	return nil
}

// fakeDocumentGetter is an in-memory stand-in for worker.DocumentGetter.
type fakeDocumentGetter struct {
	docs map[string]*model.Document
}

func (f *fakeDocumentGetter) GetByID(ctx context.Context, id string) (*model.Document, error) {
	doc, ok := f.docs[id]
	if !ok {
		return nil, errors.New("document not found")
	}
	return doc, nil
}

// fakeProcessor is an in-memory stand-in for worker.DocumentProcessor.
type fakeProcessor struct {
	chunksFor map[string][]string // document ID -> chunks to return
	err       error

	processed []string // document IDs passed to Process, in order
}

func (f *fakeProcessor) Process(ctx context.Context, doc *model.Document) ([]string, error) {
	f.processed = append(f.processed, doc.ID)
	if f.err != nil {
		return nil, f.err
	}
	return f.chunksFor[doc.ID], nil
}

// fakeChunkStore is an in-memory stand-in for worker.ChunkStore.
type fakeChunkStore struct {
	err   error
	saved [][]*model.DocumentChunk // one entry per CreateBatch call
}

func (f *fakeChunkStore) CreateBatch(ctx context.Context, chunks []*model.DocumentChunk) error {
	f.saved = append(f.saved, chunks)
	return f.err
}

func TestWorker_Run_ProcessesPendingIngestion_Success(t *testing.T) {
	ing := &model.Ingestion{ID: "ing-1", DocumentID: "doc-1", Status: model.IngestionPending}
	doc := &model.Document{ID: "doc-1", Name: "a.md"}

	ingestions := &fakeIngestionService{pending: []*model.Ingestion{ing}}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{"doc-1": doc}}
	processor := &fakeProcessor{chunksFor: map[string][]string{"doc-1": {"chunk one", "chunk two"}}}
	chunks := &fakeChunkStore{}

	w := New(ingestions, documents, processor, chunks, 0)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if len(ingestions.calls) != 2 {
		t.Fatalf("expected 2 UpdateStatus calls, got %d: %+v", len(ingestions.calls), ingestions.calls)
	}
	if got := ingestions.calls[0]; got.id != "ing-1" || got.status != model.IngestionProcessing {
		t.Errorf("expected first call to mark processing, got %+v", got)
	}
	if got := ingestions.calls[1]; got.id != "ing-1" || got.status != model.IngestionCompleted {
		t.Errorf("expected second call to mark completed, got %+v", got)
	}
	if ingestions.calls[1].errMsg != "" {
		t.Errorf("expected empty error message on success, got %q", ingestions.calls[1].errMsg)
	}

	if len(chunks.saved) != 1 {
		t.Fatalf("expected chunks to be stored exactly once, got %d calls", len(chunks.saved))
	}
	stored := chunks.saved[0]
	if len(stored) != 2 {
		t.Fatalf("expected 2 chunks stored, got %d", len(stored))
	}
	if stored[0].DocumentID != "doc-1" || stored[0].ChunkIndex != 0 || stored[0].Content != "chunk one" {
		t.Errorf("unexpected first chunk: %+v", stored[0])
	}
	if stored[1].DocumentID != "doc-1" || stored[1].ChunkIndex != 1 || stored[1].Content != "chunk two" {
		t.Errorf("unexpected second chunk: %+v", stored[1])
	}
}

func TestWorker_Run_ChunkPersistenceFails_MarksFailed(t *testing.T) {
	ing := &model.Ingestion{ID: "ing-1", DocumentID: "doc-1", Status: model.IngestionPending}
	doc := &model.Document{ID: "doc-1"}
	storeErr := errors.New("database unavailable")

	ingestions := &fakeIngestionService{pending: []*model.Ingestion{ing}}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{"doc-1": doc}}
	processor := &fakeProcessor{chunksFor: map[string][]string{"doc-1": {"a chunk"}}}
	chunkStore := &fakeChunkStore{err: storeErr}

	w := New(ingestions, documents, processor, chunkStore, 0)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if len(ingestions.calls) != 2 {
		t.Fatalf("expected 2 UpdateStatus calls, got %d: %+v", len(ingestions.calls), ingestions.calls)
	}

	final := ingestions.calls[1]
	if final.status != model.IngestionFailed {
		t.Errorf("expected final status failed when chunk persistence fails, got %s", final.status)
	}
	if final.errMsg == "" {
		t.Error("expected a non-empty error message describing the persistence failure")
	}

	// The ingestion must never have been marked completed at any point —
	// not just that the *final* status is failed, but that completed
	// never appears in the call history at all.
	for _, c := range ingestions.calls {
		if c.status == model.IngestionCompleted {
			t.Errorf("ingestion must not become completed when chunk persistence fails; got call %+v", c)
		}
	}

	// Chunks were attempted, but the ingestion must not be marked
	// completed since CreateBatch failed.
	if len(chunkStore.saved) != 1 {
		t.Errorf("expected CreateBatch to have been attempted once, got %d", len(chunkStore.saved))
	}
}

func TestWorker_Run_EmptyChunks_MarksFailed(t *testing.T) {
	ing := &model.Ingestion{ID: "ing-1", DocumentID: "doc-1", Status: model.IngestionPending}
	doc := &model.Document{ID: "doc-1"}

	ingestions := &fakeIngestionService{pending: []*model.Ingestion{ing}}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{"doc-1": doc}}
	// Processor succeeds but produces zero chunks — e.g. a document that
	// normalizes down to nothing processable.
	processor := &fakeProcessor{chunksFor: map[string][]string{"doc-1": {}}}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, chunkStore, 0)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if len(ingestions.calls) != 2 {
		t.Fatalf("expected 2 UpdateStatus calls, got %d: %+v", len(ingestions.calls), ingestions.calls)
	}

	final := ingestions.calls[1]
	if final.status != model.IngestionFailed {
		t.Errorf("expected status failed when processing produces zero chunks, got %s", final.status)
	}
	if final.errMsg == "" {
		t.Error("expected a non-empty error message explaining why it failed")
	}

	// Persisting an empty batch should never even be attempted — there's
	// nothing to persist, and reaching CreateBatch at all would risk it
	// silently no-oping and the caller mistaking that for success.
	if len(chunkStore.saved) != 0 {
		t.Errorf("expected CreateBatch not to be called for zero chunks, got %d calls", len(chunkStore.saved))
	}
}

func TestWorker_Run_ProcessingFails_MarksFailed_NoChunksStored(t *testing.T) {
	ing := &model.Ingestion{ID: "ing-1", DocumentID: "doc-1", Status: model.IngestionPending}
	doc := &model.Document{ID: "doc-1"}
	processErr := errors.New("could not normalize content")

	ingestions := &fakeIngestionService{pending: []*model.Ingestion{ing}}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{"doc-1": doc}}
	processor := &fakeProcessor{err: processErr}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, chunkStore, 0)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	final := ingestions.calls[len(ingestions.calls)-1]
	if final.status != model.IngestionFailed {
		t.Errorf("expected final status failed, got %s", final.status)
	}

	if len(chunkStore.saved) != 0 {
		t.Errorf("expected CreateBatch never called when processing fails, got %d calls", len(chunkStore.saved))
	}
}

func TestWorker_Run_DocumentFetchFails_MarksFailed(t *testing.T) {
	ing := &model.Ingestion{ID: "ing-1", DocumentID: "doc-missing", Status: model.IngestionPending}

	ingestions := &fakeIngestionService{pending: []*model.Ingestion{ing}}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{}} // doc-missing isn't there
	processor := &fakeProcessor{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, chunkStore, 0)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if len(ingestions.calls) != 2 {
		t.Fatalf("expected 2 UpdateStatus calls, got %d: %+v", len(ingestions.calls), ingestions.calls)
	}

	final := ingestions.calls[1]
	if final.status != model.IngestionFailed {
		t.Errorf("expected final status failed, got %s", final.status)
	}
	if final.errMsg == "" {
		t.Error("expected a non-empty error message describing the fetch failure")
	}

	if len(processor.processed) != 0 {
		t.Errorf("expected processor not to be called, got %v", processor.processed)
	}
	if len(chunkStore.saved) != 0 {
		t.Errorf("expected chunk store not to be called, got %d calls", len(chunkStore.saved))
	}
}

func TestWorker_Run_NoPendingIngestions_NoOp(t *testing.T) {
	ingestions := &fakeIngestionService{pending: nil}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{}}
	processor := &fakeProcessor{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, chunkStore, 0)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if len(ingestions.calls) != 0 {
		t.Errorf("expected no UpdateStatus calls, got %d", len(ingestions.calls))
	}
	if len(processor.processed) != 0 {
		t.Errorf("expected processor not to be called, got %v", processor.processed)
	}
	if len(chunkStore.saved) != 0 {
		t.Errorf("expected chunk store not to be called, got %d calls", len(chunkStore.saved))
	}
}

func TestWorker_Run_ListPendingError_ReturnsError(t *testing.T) {
	listErr := errors.New("database unavailable")

	ingestions := &fakeIngestionService{listErr: listErr}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{}}
	processor := &fakeProcessor{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, chunkStore, 0)

	err := w.Run(context.Background())
	if !errors.Is(err, listErr) {
		t.Fatalf("expected error %v, got %v", listErr, err)
	}

	if len(ingestions.calls) != 0 {
		t.Errorf("expected no UpdateStatus calls when listing pending fails, got %d", len(ingestions.calls))
	}
}

func TestWorker_Run_MultipleIngestions_OneFailureDoesNotStopBatch(t *testing.T) {
	okIng := &model.Ingestion{ID: "ing-ok", DocumentID: "doc-ok", Status: model.IngestionPending}
	badIng := &model.Ingestion{ID: "ing-bad", DocumentID: "doc-bad", Status: model.IngestionPending}

	docOK := &model.Document{ID: "doc-ok"}
	docBad := &model.Document{ID: "doc-bad"}

	ingestions := &fakeIngestionService{pending: []*model.Ingestion{badIng, okIng}}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{
		"doc-ok":  docOK,
		"doc-bad": docBad,
	}}

	processor := &conditionalProcessor{
		failFor:   map[string]error{"doc-bad": errors.New("processing failed")},
		chunksFor: map[string][]string{"doc-ok": {"a chunk"}},
	}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, chunkStore, 0)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	// 2 UpdateStatus calls per ingestion (processing + terminal) x 2 ingestions = 4.
	if len(ingestions.calls) != 4 {
		t.Fatalf("expected 4 UpdateStatus calls, got %d: %+v", len(ingestions.calls), ingestions.calls)
	}

	var badFinal, okFinal *statusCall
	for i := range ingestions.calls {
		c := ingestions.calls[i]
		if c.id == "ing-bad" && (c.status == model.IngestionFailed || c.status == model.IngestionCompleted) {
			badFinal = &ingestions.calls[i]
		}
		if c.id == "ing-ok" && (c.status == model.IngestionFailed || c.status == model.IngestionCompleted) {
			okFinal = &ingestions.calls[i]
		}
	}

	if badFinal == nil || badFinal.status != model.IngestionFailed {
		t.Errorf("expected ing-bad to end failed, got %+v", badFinal)
	}
	if okFinal == nil || okFinal.status != model.IngestionCompleted {
		t.Errorf("expected ing-ok to end completed despite ing-bad failing, got %+v", okFinal)
	}

	// Only the successful ingestion's chunks should have been stored.
	if len(chunkStore.saved) != 1 {
		t.Fatalf("expected exactly 1 CreateBatch call (for the successful ingestion), got %d", len(chunkStore.saved))
	}
}

func TestWorker_Run_RespectsBatchSize(t *testing.T) {
	pending := []*model.Ingestion{
		{ID: "ing-1", DocumentID: "doc-1", Status: model.IngestionPending},
		{ID: "ing-2", DocumentID: "doc-2", Status: model.IngestionPending},
		{ID: "ing-3", DocumentID: "doc-3", Status: model.IngestionPending},
	}

	ingestions := &fakeIngestionService{pending: pending}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{
		"doc-1": {ID: "doc-1"},
		"doc-2": {ID: "doc-2"},
		"doc-3": {ID: "doc-3"},
	}}
	processor := &fakeProcessor{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, chunkStore, 2)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	// fakeIngestionService.ListPending already truncates to the requested
	// limit, matching real repository behavior (SQL LIMIT). With batch
	// size 2, only 2 ingestions should have been processed.
	if len(processor.processed) != 2 {
		t.Errorf("expected 2 documents processed with batch size 2, got %d", len(processor.processed))
	}
}

func TestWorker_New_DefaultsBatchSizeWhenZeroOrNegative(t *testing.T) {
	ingestions := &fakeIngestionService{}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{}}
	processor := &fakeProcessor{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, chunkStore, 0)
	if w.batchSize != defaultBatchSize {
		t.Errorf("expected default batch size %d, got %d", defaultBatchSize, w.batchSize)
	}

	w = New(ingestions, documents, processor, chunkStore, -5)
	if w.batchSize != defaultBatchSize {
		t.Errorf("expected default batch size %d for negative input, got %d", defaultBatchSize, w.batchSize)
	}
}

// conditionalProcessor fails only for specific document IDs, letting a
// test simulate one bad document among several good ones.
type conditionalProcessor struct {
	failFor   map[string]error
	chunksFor map[string][]string
}

func (p *conditionalProcessor) Process(ctx context.Context, doc *model.Document) ([]string, error) {
	if err, ok := p.failFor[doc.ID]; ok {
		return nil, err
	}
	return p.chunksFor[doc.ID], nil
}
