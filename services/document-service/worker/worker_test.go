package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

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

	calls         []statusCall
	listCallCount int
}

func (f *fakeIngestionService) ListPending(ctx context.Context, limit int) ([]*model.Ingestion, error) {
	f.listCallCount++
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

// fakeEmbedder is an in-memory stand-in for embedding.Embedder.
type fakeEmbedder struct {
	err         error
	vectorCount int // if > 0, overrides len(texts) — lets a test simulate a mismatched count
	dims        int
	calledWith  [][]string // each call's input texts, in order
}

func (f *fakeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	f.calledWith = append(f.calledWith, texts)
	if f.err != nil {
		return nil, f.err
	}

	count := len(texts)
	if f.vectorCount > 0 {
		count = f.vectorCount
	}

	dims := f.dims
	if dims <= 0 {
		dims = 4
	}

	vectors := make([][]float32, count)
	for i := range vectors {
		vectors[i] = make([]float32, dims)
	}
	return vectors, nil
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
	embedder := &fakeEmbedder{}
	chunks := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunks, 0)

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

func TestWorker_Run_EmbeddingsReachChunkStore(t *testing.T) {
	ing := &model.Ingestion{ID: "ing-1", DocumentID: "doc-1", Status: model.IngestionPending}
	doc := &model.Document{ID: "doc-1"}

	ingestions := &fakeIngestionService{pending: []*model.Ingestion{ing}}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{"doc-1": doc}}
	processor := &fakeProcessor{chunksFor: map[string][]string{"doc-1": {"chunk one", "chunk two"}}}
	embedder := &fakeEmbedder{dims: 8}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if len(chunkStore.saved) != 1 {
		t.Fatalf("expected exactly 1 CreateBatch call, got %d", len(chunkStore.saved))
	}

	stored := chunkStore.saved[0]
	if len(stored) != 2 {
		t.Fatalf("expected 2 chunks stored, got %d", len(stored))
	}

	for i, c := range stored {
		if c.Embedding == nil {
			t.Errorf("chunk %d: expected a non-nil embedding, got nil", i)
		}
		if len(c.Embedding) != 8 {
			t.Errorf("chunk %d: expected embedding of length 8, got %d", i, len(c.Embedding))
		}
	}
}

func TestWorker_Run_EmbedsChunks_CalledWithChunkTexts(t *testing.T) {
	ing := &model.Ingestion{ID: "ing-1", DocumentID: "doc-1", Status: model.IngestionPending}
	doc := &model.Document{ID: "doc-1"}

	ingestions := &fakeIngestionService{pending: []*model.Ingestion{ing}}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{"doc-1": doc}}
	processor := &fakeProcessor{chunksFor: map[string][]string{"doc-1": {"alpha", "beta", "gamma"}}}
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if len(embedder.calledWith) != 1 {
		t.Fatalf("expected Embed to be called once, got %d calls", len(embedder.calledWith))
	}

	got := embedder.calledWith[0]
	want := []string{"alpha", "beta", "gamma"}
	if len(got) != len(want) {
		t.Fatalf("expected %d texts passed to Embed, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d: expected %q, got %q", i, want[i], got[i])
		}
	}
}

func TestWorker_Run_EmbeddingFails_MarksFailed_ChunksNeverStored(t *testing.T) {
	ing := &model.Ingestion{ID: "ing-1", DocumentID: "doc-1", Status: model.IngestionPending}
	doc := &model.Document{ID: "doc-1"}
	embedErr := errors.New("embedding provider unavailable")

	ingestions := &fakeIngestionService{pending: []*model.Ingestion{ing}}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{"doc-1": doc}}
	processor := &fakeProcessor{chunksFor: map[string][]string{"doc-1": {"a chunk"}}}
	embedder := &fakeEmbedder{err: embedErr}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if len(ingestions.calls) != 2 {
		t.Fatalf("expected 2 UpdateStatus calls, got %d: %+v", len(ingestions.calls), ingestions.calls)
	}

	final := ingestions.calls[1]
	if final.status != model.IngestionFailed {
		t.Errorf("expected final status failed when embedding fails, got %s", final.status)
	}
	if final.errMsg == "" {
		t.Error("expected a non-empty error message describing the embedding failure")
	}

	if len(chunkStore.saved) != 0 {
		t.Errorf("expected chunks never persisted when embedding fails, got %d CreateBatch calls", len(chunkStore.saved))
	}
}

func TestWorker_Run_EmbeddingCountMismatch_MarksFailed(t *testing.T) {
	ing := &model.Ingestion{ID: "ing-1", DocumentID: "doc-1", Status: model.IngestionPending}
	doc := &model.Document{ID: "doc-1"}

	ingestions := &fakeIngestionService{pending: []*model.Ingestion{ing}}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{"doc-1": doc}}
	// 3 chunks produced, but the (misbehaving) embedder returns only 2 vectors.
	processor := &fakeProcessor{chunksFor: map[string][]string{"doc-1": {"a", "b", "c"}}}
	embedder := &fakeEmbedder{vectorCount: 2}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	final := ingestions.calls[len(ingestions.calls)-1]
	if final.status != model.IngestionFailed {
		t.Errorf("expected status failed on embedding count mismatch, got %s", final.status)
	}
	if final.errMsg == "" {
		t.Error("expected a non-empty error message explaining the mismatch")
	}

	if len(chunkStore.saved) != 0 {
		t.Errorf("expected chunks never persisted on embedding count mismatch, got %d calls", len(chunkStore.saved))
	}
}

func TestWorker_Run_ChunkPersistenceFails_MarksFailed(t *testing.T) {
	ing := &model.Ingestion{ID: "ing-1", DocumentID: "doc-1", Status: model.IngestionPending}
	doc := &model.Document{ID: "doc-1"}
	storeErr := errors.New("database unavailable")

	ingestions := &fakeIngestionService{pending: []*model.Ingestion{ing}}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{"doc-1": doc}}
	processor := &fakeProcessor{chunksFor: map[string][]string{"doc-1": {"a chunk"}}}
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{err: storeErr}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0)

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

	for _, c := range ingestions.calls {
		if c.status == model.IngestionCompleted {
			t.Errorf("ingestion must not become completed when chunk persistence fails; got call %+v", c)
		}
	}

	if len(chunkStore.saved) != 1 {
		t.Errorf("expected CreateBatch to have been attempted once, got %d", len(chunkStore.saved))
	}
}

func TestWorker_Run_EmptyChunks_MarksFailed(t *testing.T) {
	ing := &model.Ingestion{ID: "ing-1", DocumentID: "doc-1", Status: model.IngestionPending}
	doc := &model.Document{ID: "doc-1"}

	ingestions := &fakeIngestionService{pending: []*model.Ingestion{ing}}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{"doc-1": doc}}
	processor := &fakeProcessor{chunksFor: map[string][]string{"doc-1": {}}}
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0)

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

	if len(embedder.calledWith) != 0 {
		t.Errorf("expected Embed not to be called for zero chunks, got %d calls", len(embedder.calledWith))
	}
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
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	final := ingestions.calls[len(ingestions.calls)-1]
	if final.status != model.IngestionFailed {
		t.Errorf("expected final status failed, got %s", final.status)
	}

	if len(embedder.calledWith) != 0 {
		t.Errorf("expected Embed never called when processing fails, got %d calls", len(embedder.calledWith))
	}
	if len(chunkStore.saved) != 0 {
		t.Errorf("expected CreateBatch never called when processing fails, got %d calls", len(chunkStore.saved))
	}
}

func TestWorker_Run_DocumentFetchFails_MarksFailed(t *testing.T) {
	ing := &model.Ingestion{ID: "ing-1", DocumentID: "doc-missing", Status: model.IngestionPending}

	ingestions := &fakeIngestionService{pending: []*model.Ingestion{ing}}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{}}
	processor := &fakeProcessor{}
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0)

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
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0)

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
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0)

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
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

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
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 2)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if len(processor.processed) != 2 {
		t.Errorf("expected 2 documents processed with batch size 2, got %d", len(processor.processed))
	}
}

func TestWorker_New_DefaultsBatchSizeWhenZeroOrNegative(t *testing.T) {
	ingestions := &fakeIngestionService{}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{}}
	processor := &fakeProcessor{}
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0)
	if w.batchSize != defaultBatchSize {
		t.Errorf("expected default batch size %d, got %d", defaultBatchSize, w.batchSize)
	}

	w = New(ingestions, documents, processor, embedder, chunkStore, -5)
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

func TestWorker_RunLoop_PollsRepeatedlyUntilCancelled(t *testing.T) {
	ingestions := &fakeIngestionService{}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{}}
	processor := &fakeProcessor{}
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- w.RunLoop(ctx, 5*time.Millisecond, nil)
	}()

	// Let several ticks elapse, then stop the loop.
	time.Sleep(30 * time.Millisecond)
	cancel()

	// Wait for RunLoop to actually return — this happens-before
	// relationship (channel receive) is what makes reading
	// ingestions.listCallCount afterward race-free, since RunLoop's
	// goroutine is guaranteed to have finished writing to it by the
	// time we read.
	err := <-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected RunLoop to return context.Canceled, got %v", err)
	}

	if ingestions.listCallCount < 2 {
		t.Errorf("expected RunLoop to poll more than once in 30ms with a 5ms interval, got %d calls", ingestions.listCallCount)
	}
}

func TestWorker_RunLoop_RunsImmediatelyOnStart(t *testing.T) {
	ingestions := &fakeIngestionService{}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{}}
	processor := &fakeProcessor{}
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{}

	// A long interval — if RunLoop waited for the first tick before
	// doing anything, listCallCount would still be 0 by the time we
	// cancel almost immediately below.
	w := New(ingestions, documents, processor, embedder, chunkStore, 0)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- w.RunLoop(ctx, 1*time.Hour, nil)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()
	<-done

	if ingestions.listCallCount < 1 {
		t.Error("expected RunLoop to call Run immediately on start, before waiting for the first tick")
	}
}

func TestWorker_RunLoop_ReportsErrorsButKeepsPolling(t *testing.T) {
	ingestions := &fakeIngestionService{listErr: errors.New("transient database error")}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{}}
	processor := &fakeProcessor{}
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0)

	var errCount int
	var mu sync.Mutex
	onError := func(err error) {
		mu.Lock()
		errCount++
		mu.Unlock()
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- w.RunLoop(ctx, 5*time.Millisecond, onError)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if errCount < 2 {
		t.Errorf("expected onError to be called more than once as the loop kept retrying, got %d calls", errCount)
	}
	// The loop must have kept going despite every pass failing — proven
	// by listCallCount tracking multiple attempts, not just one.
	if ingestions.listCallCount < 2 {
		t.Errorf("expected RunLoop to keep polling despite errors, got %d calls", ingestions.listCallCount)
	}
}

func TestWorker_RunLoop_DefaultsIntervalWhenZero(t *testing.T) {
	// This test only confirms RunLoop doesn't panic/misbehave with
	// interval=0 (defaults internally to defaultPollInterval) — it
	// doesn't wait out a real 5s interval, just confirms the immediate
	// on-start Run still happens and the loop is cancellable promptly.
	ingestions := &fakeIngestionService{}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{}}
	processor := &fakeProcessor{}
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- w.RunLoop(ctx, 0, nil)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("RunLoop did not return promptly after context cancellation")
	}

	if ingestions.listCallCount < 1 {
		t.Error("expected at least the immediate on-start Run to have happened")
	}
}
