package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
// It simulates the real repository's ClaimNextPending semantics: claiming
// atomically finds the first ingestion still in "pending" status, flips it
// to "processing", and increments its Attempts — all under a mutex, so
// this fake is safe to share across goroutines in a concurrency test the
// same way the real FOR UPDATE SKIP LOCKED query is safe across real
// concurrent transactions.
type fakeIngestionService struct {
	mu      sync.Mutex
	pending []*model.Ingestion
	listErr error // returned by ClaimNextPending when set

	calls         []statusCall
	listCallCount int // counts ClaimNextPending calls
}

func (f *fakeIngestionService) ClaimNextPending(ctx context.Context) (*model.Ingestion, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.listCallCount++

	if f.listErr != nil {
		return nil, f.listErr
	}

	for _, ing := range f.pending {
		if ing.Status == model.IngestionPending {
			ing.Attempts++
			ing.Status = model.IngestionProcessing
			return ing, nil
		}
	}

	return nil, nil
}

func (f *fakeIngestionService) UpdateStatus(ctx context.Context, id string, status model.IngestionStatus, errMsg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, statusCall{id: id, status: status, errMsg: errMsg})

	for _, ing := range f.pending {
		if ing.ID == id {
			ing.Status = status
			if errMsg != "" {
				ing.LastError = errMsg
			}
		}
	}

	return nil
}

// callsFor returns every recorded UpdateStatus call for a given ingestion
// ID, in call order — useful for asserting a specific ingestion's status
// history (e.g. "went pending -> processing -> pending -> processing ->
// failed" across retries) without the noise of other ingestions' calls
// mixed in.
func (f *fakeIngestionService) callsFor(id string) []statusCall {
	f.mu.Lock()
	defer f.mu.Unlock()

	var out []statusCall
	for _, c := range f.calls {
		if c.id == id {
			out = append(out, c)
		}
	}
	return out
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

	mu        sync.Mutex
	processed []string // document IDs passed to Process, in order
}

func (f *fakeProcessor) Process(ctx context.Context, doc *model.Document) ([]string, error) {
	f.mu.Lock()
	f.processed = append(f.processed, doc.ID)
	f.mu.Unlock()

	if f.err != nil {
		return nil, f.err
	}
	return f.chunksFor[doc.ID], nil
}

func (f *fakeProcessor) processedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.processed)
}

// fakeEmbedder is an in-memory stand-in for embedding.Embedder.
type fakeEmbedder struct {
	err         error
	vectorCount int // if > 0, overrides len(texts) — simulates a mismatched count
	dims        int

	mu         sync.Mutex
	calledWith [][]string
}

func (f *fakeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	f.mu.Lock()
	f.calledWith = append(f.calledWith, texts)
	f.mu.Unlock()

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
	err error

	mu    sync.Mutex
	saved [][]*model.DocumentChunk
}

func (f *fakeChunkStore) CreateBatch(ctx context.Context, chunks []*model.DocumentChunk) error {
	f.mu.Lock()
	f.saved = append(f.saved, chunks)
	f.mu.Unlock()
	return f.err
}

func (f *fakeChunkStore) savedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.saved)
}

// --- Required tests (Phase 7, task item 6) -----------------------------

func TestWorker_ProcessesPendingIngestion(t *testing.T) {
	ing := &model.Ingestion{ID: "ing-1", DocumentID: "doc-1", Status: model.IngestionPending}
	doc := &model.Document{ID: "doc-1"}

	ingestions := &fakeIngestionService{pending: []*model.Ingestion{ing}}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{"doc-1": doc}}
	processor := &fakeProcessor{chunksFor: map[string][]string{"doc-1": {"chunk one", "chunk two"}}}
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0, 0)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if ing.Status != model.IngestionCompleted {
		t.Errorf("expected ingestion completed, got %s", ing.Status)
	}
	if ing.Attempts != 1 {
		t.Errorf("expected 1 attempt recorded, got %d", ing.Attempts)
	}
	if chunkStore.savedCount() != 1 {
		t.Fatalf("expected chunks stored once, got %d calls", chunkStore.savedCount())
	}
}

func TestWorker_RetriesFailedIngestion(t *testing.T) {
	ing := &model.Ingestion{ID: "ing-1", DocumentID: "doc-1", Status: model.IngestionPending}
	doc := &model.Document{ID: "doc-1"}

	ingestions := &fakeIngestionService{pending: []*model.Ingestion{ing}}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{"doc-1": doc}}
	// Fails every time — we're only checking the retry transition here,
	// not eventual success.
	processor := &fakeProcessor{err: errors.New("transient processing error")}
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0, 3) // maxAttempts=3

	// First pass: attempt 1 of 3 fails -> should go back to "pending",
	// not "failed".
	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if ing.Status != model.IngestionPending {
		t.Fatalf("expected ingestion back to pending after attempt 1/3, got %s", ing.Status)
	}
	if ing.Attempts != 1 {
		t.Fatalf("expected 1 attempt recorded, got %d", ing.Attempts)
	}
	if ing.LastError == "" {
		t.Error("expected LastError to be recorded even on a retry")
	}

	calls := ingestions.callsFor("ing-1")
	if len(calls) != 1 || calls[0].status != model.IngestionPending {
		t.Errorf("expected exactly one UpdateStatus call moving back to pending, got %+v", calls)
	}
}

func TestWorker_MarksFailedAfterMaxAttempts(t *testing.T) {
	// Attempts already at 2, about to become 3 on this claim — with
	// maxAttempts=3, this is the LAST allowed attempt, so a failure here
	// must result in permanent "failed", not another retry.
	ing := &model.Ingestion{ID: "ing-1", DocumentID: "doc-1", Status: model.IngestionPending, Attempts: 2}
	doc := &model.Document{ID: "doc-1"}

	ingestions := &fakeIngestionService{pending: []*model.Ingestion{ing}}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{"doc-1": doc}}
	processor := &fakeProcessor{err: errors.New("permanent-looking processing error")}
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0, 3)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if ing.Attempts != 3 {
		t.Fatalf("expected attempts incremented to 3, got %d", ing.Attempts)
	}
	if ing.Status != model.IngestionFailed {
		t.Fatalf("expected ingestion permanently failed after exhausting attempts, got %s", ing.Status)
	}

	calls := ingestions.callsFor("ing-1")
	if len(calls) != 1 || calls[0].status != model.IngestionFailed {
		t.Errorf("expected exactly one UpdateStatus call moving to failed, got %+v", calls)
	}
}

func TestWorker_DoesNotProcessCompletedIngestion(t *testing.T) {
	completed := &model.Ingestion{ID: "ing-done", DocumentID: "doc-1", Status: model.IngestionCompleted}
	pending := &model.Ingestion{ID: "ing-pending", DocumentID: "doc-2", Status: model.IngestionPending}

	ingestions := &fakeIngestionService{pending: []*model.Ingestion{completed, pending}}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{
		"doc-1": {ID: "doc-1"},
		"doc-2": {ID: "doc-2"},
	}}
	processor := &fakeProcessor{chunksFor: map[string][]string{"doc-2": {"a chunk"}}}
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0, 0)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	// The completed ingestion must be untouched: no new UpdateStatus
	// calls for it, no attempts incremented, and the processor must
	// never have been asked to process its document.
	if completed.Attempts != 0 {
		t.Errorf("expected completed ingestion's attempts untouched, got %d", completed.Attempts)
	}
	if len(ingestions.callsFor("ing-done")) != 0 {
		t.Errorf("expected no UpdateStatus calls for the already-completed ingestion")
	}

	found := false
	for _, id := range processor.processed {
		if id == "doc-1" {
			found = true
		}
	}
	if found {
		t.Error("expected doc-1 (belonging to the completed ingestion) never to be processed")
	}

	// Meanwhile the genuinely pending one should have gone through
	// normally.
	if pending.Status != model.IngestionCompleted {
		t.Errorf("expected the pending ingestion to complete normally, got %s", pending.Status)
	}
}

func TestWorker_DoesNotProcessFailedIngestion(t *testing.T) {
	failed := &model.Ingestion{ID: "ing-failed", DocumentID: "doc-1", Status: model.IngestionFailed, Attempts: 3}
	pending := &model.Ingestion{ID: "ing-pending", DocumentID: "doc-2", Status: model.IngestionPending}

	ingestions := &fakeIngestionService{pending: []*model.Ingestion{failed, pending}}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{
		"doc-1": {ID: "doc-1"},
		"doc-2": {ID: "doc-2"},
	}}
	processor := &fakeProcessor{chunksFor: map[string][]string{"doc-2": {"a chunk"}}}
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0, 0)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	// A permanently failed ingestion must never be picked up again —
	// ClaimNextPending only ever claims rows still in "pending", and
	// failed is a terminal state, not something the worker retries on
	// its own. (Re-driving a failed job would need an explicit
	// requeue-to-pending action, which doesn't exist yet.)
	if failed.Attempts != 3 {
		t.Errorf("expected failed ingestion's attempts untouched, got %d", failed.Attempts)
	}
	if len(ingestions.callsFor("ing-failed")) != 0 {
		t.Errorf("expected no UpdateStatus calls for the already-failed ingestion")
	}

	found := false
	for _, id := range processor.processed {
		if id == "doc-1" {
			found = true
		}
	}
	if found {
		t.Error("expected doc-1 (belonging to the failed ingestion) never to be processed")
	}

	if pending.Status != model.IngestionCompleted {
		t.Errorf("expected the pending ingestion to complete normally, got %s", pending.Status)
	}
}

// --- Explicit retry-behavior tests (this task) --------------------------
//
// These pin down the exact state machine:
//
//	pending -> claim -> processing -> success -> completed
//	pending -> claim -> processing -> failure -> attempts < MAX -> pending
//	pending -> claim -> processing -> failure -> attempts >= MAX -> failed (+ last_error)

func TestWorker_RetryOnProcessingFailure(t *testing.T) {
	ing := &model.Ingestion{ID: "ing-1", DocumentID: "doc-1", Status: model.IngestionPending}
	doc := &model.Document{ID: "doc-1"}

	ingestions := &fakeIngestionService{pending: []*model.Ingestion{ing}}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{"doc-1": doc}}
	processor := &fakeProcessor{err: errors.New("processing blew up")}
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0, 3) // MAX_ATTEMPTS=3

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	// A single failed attempt with attempts (1) still below MAX (3) must
	// land back on "pending", never "failed" or "completed".
	if ing.Status != model.IngestionPending {
		t.Fatalf("expected ingestion requeued to pending after a failed attempt, got %s", ing.Status)
	}
}

func TestWorker_RequeuesBeforeMaxAttempts(t *testing.T) {
	ing := &model.Ingestion{ID: "ing-1", DocumentID: "doc-1", Status: model.IngestionPending}
	doc := &model.Document{ID: "doc-1"}

	ingestions := &fakeIngestionService{pending: []*model.Ingestion{ing}}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{"doc-1": doc}}
	// Fails exactly once, then succeeds — proves a requeued ingestion is
	// genuinely re-claimable on a LATER Run() pass, not lost or stuck.
	processor := &conditionalFailNTimesProcessor{failTimes: 1, chunksFor: map[string][]string{"doc-1": {"a chunk"}}}
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0, 3)

	// Pass 1: fails, attempts=1 < 3 -> back to pending.
	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() pass 1 returned error: %v", err)
	}
	if ing.Status != model.IngestionPending {
		t.Fatalf("expected pending after pass 1, got %s", ing.Status)
	}
	if ing.Attempts != 1 {
		t.Fatalf("expected 1 attempt after pass 1, got %d", ing.Attempts)
	}

	// Pass 2 (simulating the next RunLoop tick): the requeued ingestion
	// must be claimable again — this is the "before max attempts ->
	// pending again" half of the state machine actually working across
	// separate polling passes, not just within one.
	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() pass 2 returned error: %v", err)
	}
	if ing.Status != model.IngestionCompleted {
		t.Fatalf("expected completed after pass 2 (processor succeeds on 2nd attempt), got %s", ing.Status)
	}
	if ing.Attempts != 2 {
		t.Fatalf("expected 2 attempts total, got %d", ing.Attempts)
	}
}

func TestWorker_RecordsLastError(t *testing.T) {
	t.Run("on retry", func(t *testing.T) {
		ing := &model.Ingestion{ID: "ing-1", DocumentID: "doc-1", Status: model.IngestionPending}
		doc := &model.Document{ID: "doc-1"}

		ingestions := &fakeIngestionService{pending: []*model.Ingestion{ing}}
		documents := &fakeDocumentGetter{docs: map[string]*model.Document{"doc-1": doc}}
		processor := &fakeProcessor{err: errors.New("specific retry failure reason")}
		embedder := &fakeEmbedder{}
		chunkStore := &fakeChunkStore{}

		w := New(ingestions, documents, processor, embedder, chunkStore, 0, 3)

		if err := w.Run(context.Background()); err != nil {
			t.Fatalf("Run() returned error: %v", err)
		}

		if ing.LastError == "" {
			t.Fatal("expected LastError to be recorded on a retry, got empty string")
		}
		if !strings.Contains(ing.LastError, "specific retry failure reason") {
			t.Errorf("expected LastError to contain the underlying failure reason, got %q", ing.LastError)
		}
	})

	t.Run("on permanent failure", func(t *testing.T) {
		ing := &model.Ingestion{ID: "ing-1", DocumentID: "doc-1", Status: model.IngestionPending, Attempts: 2}
		doc := &model.Document{ID: "doc-1"}

		ingestions := &fakeIngestionService{pending: []*model.Ingestion{ing}}
		documents := &fakeDocumentGetter{docs: map[string]*model.Document{"doc-1": doc}}
		processor := &fakeProcessor{err: errors.New("specific permanent failure reason")}
		embedder := &fakeEmbedder{}
		chunkStore := &fakeChunkStore{}

		w := New(ingestions, documents, processor, embedder, chunkStore, 0, 3) // this claim -> attempts=3 -> exhausted

		if err := w.Run(context.Background()); err != nil {
			t.Fatalf("Run() returned error: %v", err)
		}

		if ing.Status != model.IngestionFailed {
			t.Fatalf("expected permanently failed, got %s", ing.Status)
		}
		if !strings.Contains(ing.LastError, "specific permanent failure reason") {
			t.Errorf("expected LastError to contain the underlying failure reason, got %q", ing.LastError)
		}
	})
}

func TestWorker_SuccessPreservesCorrectAttemptState(t *testing.T) {
	// Simulate an ingestion that already failed once before (as if from
	// a previous Run() pass): attempts=1, LastError set from that
	// earlier failure, status back to pending (requeued).
	ing := &model.Ingestion{
		ID:         "ing-1",
		DocumentID: "doc-1",
		Status:     model.IngestionPending,
		Attempts:   1,
		LastError:  "earlier transient failure",
	}
	doc := &model.Document{ID: "doc-1"}

	ingestions := &fakeIngestionService{pending: []*model.Ingestion{ing}}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{"doc-1": doc}}
	processor := &fakeProcessor{chunksFor: map[string][]string{"doc-1": {"a chunk"}}} // succeeds this time
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0, 3)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if ing.Status != model.IngestionCompleted {
		t.Fatalf("expected completed on success, got %s", ing.Status)
	}
	// This claim increments attempts from 1 -> 2. Success does NOT reset
	// attempts back to 0 — attempts is a lifetime counter of how many
	// times this ingestion was ever claimed, not "attempts since last
	// failure". A history of "it took 2 tries" is useful information to
	// keep, not something success should erase.
	if ing.Attempts != 2 {
		t.Errorf("expected attempts preserved at 2 (1 prior + 1 this claim), got %d", ing.Attempts)
	}
	// LastError is sticky by design (see repository.IngestionRepo.UpdateStatus's
	// doc comment) — a successful completion doesn't erase the record
	// that an earlier attempt failed; it stays as history.
	if ing.LastError != "earlier transient failure" {
		t.Errorf("expected LastError preserved from the earlier failure (sticky, not cleared on success), got %q", ing.LastError)
	}
}

func TestWorker_ConcurrentWorkersDoNotDuplicate(t *testing.T) {
	const numIngestions = 20

	ingestionList := make([]*model.Ingestion, numIngestions)
	docs := map[string]*model.Document{}
	chunksFor := map[string][]string{}

	for i := 0; i < numIngestions; i++ {
		id := fmt.Sprintf("ing-%d", i)
		docID := fmt.Sprintf("doc-%d", i)
		ingestionList[i] = &model.Ingestion{ID: id, DocumentID: docID, Status: model.IngestionPending}
		docs[docID] = &model.Document{ID: docID}
		chunksFor[docID] = []string{"a chunk"}
	}

	// A single shared fakeIngestionService, shared fakeChunkStore, etc —
	// simulating multiple Worker instances (e.g. multiple replicas of
	// the same service) all pointed at the same underlying store, the
	// same way multiple real worker processes would all point at the
	// same PostgreSQL database.
	ingestions := &fakeIngestionService{pending: ingestionList}
	documents := &fakeDocumentGetter{docs: docs}
	processor := &fakeProcessor{chunksFor: chunksFor}
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{}

	const numWorkers = 5
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		w := New(ingestions, documents, processor, embedder, chunkStore, numIngestions, 0)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := w.Run(context.Background()); err != nil {
				t.Errorf("Run() returned error: %v", err)
			}
		}()
	}
	wg.Wait()

	// Every ingestion must have completed exactly once — not zero times
	// (would mean some were never claimed) and not more than once (would
	// mean two workers both processed the same one, the exact bug this
	// whole mechanism exists to prevent).
	for _, ing := range ingestionList {
		if ing.Status != model.IngestionCompleted {
			t.Errorf("ingestion %s: expected completed, got %s", ing.ID, ing.Status)
		}
		if ing.Attempts != 1 {
			t.Errorf("ingestion %s: expected exactly 1 attempt, got %d — a duplicate claim would show up here", ing.ID, ing.Attempts)
		}

		completedCalls := 0
		for _, c := range ingestions.callsFor(ing.ID) {
			if c.status == model.IngestionCompleted {
				completedCalls++
			}
		}
		if completedCalls != 1 {
			t.Errorf("ingestion %s: expected exactly 1 completed UpdateStatus call, got %d", ing.ID, completedCalls)
		}
	}

	if processor.processedCount() != numIngestions {
		t.Errorf("expected each of %d documents processed exactly once, got %d Process() calls", numIngestions, processor.processedCount())
	}
	if chunkStore.savedCount() != numIngestions {
		t.Errorf("expected exactly %d CreateBatch calls (one per ingestion), got %d", numIngestions, chunkStore.savedCount())
	}
}

// --- Additional coverage (from earlier phases, adapted to the new
//     claim-based fake) -------------------------------------------------

func TestWorker_Run_EmbedsChunks_CalledWithChunkTexts(t *testing.T) {
	ing := &model.Ingestion{ID: "ing-1", DocumentID: "doc-1", Status: model.IngestionPending}
	doc := &model.Document{ID: "doc-1"}

	ingestions := &fakeIngestionService{pending: []*model.Ingestion{ing}}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{"doc-1": doc}}
	processor := &fakeProcessor{chunksFor: map[string][]string{"doc-1": {"alpha", "beta", "gamma"}}}
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0, 0)

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
	ing := &model.Ingestion{ID: "ing-1", DocumentID: "doc-1", Status: model.IngestionPending, Attempts: 2}
	doc := &model.Document{ID: "doc-1"}
	embedErr := errors.New("embedding provider unavailable")

	ingestions := &fakeIngestionService{pending: []*model.Ingestion{ing}}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{"doc-1": doc}}
	processor := &fakeProcessor{chunksFor: map[string][]string{"doc-1": {"a chunk"}}}
	embedder := &fakeEmbedder{err: embedErr}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0, 3) // this claim -> attempts=3 -> exhausted

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if ing.Status != model.IngestionFailed {
		t.Errorf("expected final status failed when embedding fails and attempts exhausted, got %s", ing.Status)
	}
	if chunkStore.savedCount() != 0 {
		t.Errorf("expected chunks never persisted when embedding fails, got %d CreateBatch calls", chunkStore.savedCount())
	}
}

func TestWorker_Run_EmbeddingCountMismatch_MarksFailed(t *testing.T) {
	ing := &model.Ingestion{ID: "ing-1", DocumentID: "doc-1", Status: model.IngestionPending, Attempts: 2}
	doc := &model.Document{ID: "doc-1"}

	ingestions := &fakeIngestionService{pending: []*model.Ingestion{ing}}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{"doc-1": doc}}
	processor := &fakeProcessor{chunksFor: map[string][]string{"doc-1": {"a", "b", "c"}}}
	embedder := &fakeEmbedder{vectorCount: 2} // 3 chunks, only 2 vectors back
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0, 3)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if ing.Status != model.IngestionFailed {
		t.Errorf("expected status failed on embedding count mismatch with attempts exhausted, got %s", ing.Status)
	}
	if chunkStore.savedCount() != 0 {
		t.Errorf("expected chunks never persisted on embedding count mismatch, got %d calls", chunkStore.savedCount())
	}
}

func TestWorker_Run_ChunkPersistenceFails_MarksFailed(t *testing.T) {
	ing := &model.Ingestion{ID: "ing-1", DocumentID: "doc-1", Status: model.IngestionPending, Attempts: 2}
	doc := &model.Document{ID: "doc-1"}
	storeErr := errors.New("database unavailable")

	ingestions := &fakeIngestionService{pending: []*model.Ingestion{ing}}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{"doc-1": doc}}
	processor := &fakeProcessor{chunksFor: map[string][]string{"doc-1": {"a chunk"}}}
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{err: storeErr}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0, 3)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if ing.Status != model.IngestionFailed {
		t.Errorf("expected final status failed when chunk persistence fails and attempts exhausted, got %s", ing.Status)
	}

	for _, c := range ingestions.callsFor("ing-1") {
		if c.status == model.IngestionCompleted {
			t.Errorf("ingestion must not become completed when chunk persistence fails; got call %+v", c)
		}
	}
	if chunkStore.savedCount() != 1 {
		t.Errorf("expected CreateBatch to have been attempted once, got %d", chunkStore.savedCount())
	}
}

func TestWorker_Run_EmptyChunks_MarksFailed(t *testing.T) {
	ing := &model.Ingestion{ID: "ing-1", DocumentID: "doc-1", Status: model.IngestionPending, Attempts: 2}
	doc := &model.Document{ID: "doc-1"}

	ingestions := &fakeIngestionService{pending: []*model.Ingestion{ing}}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{"doc-1": doc}}
	processor := &fakeProcessor{chunksFor: map[string][]string{"doc-1": {}}}
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0, 3)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if ing.Status != model.IngestionFailed {
		t.Errorf("expected status failed when processing produces zero chunks and attempts exhausted, got %s", ing.Status)
	}
	if len(embedder.calledWith) != 0 {
		t.Errorf("expected Embed not to be called for zero chunks, got %d calls", len(embedder.calledWith))
	}
	if chunkStore.savedCount() != 0 {
		t.Errorf("expected CreateBatch not to be called for zero chunks, got %d calls", chunkStore.savedCount())
	}
}

func TestWorker_Run_DocumentFetchFails_MarksFailed(t *testing.T) {
	ing := &model.Ingestion{ID: "ing-1", DocumentID: "doc-missing", Status: model.IngestionPending, Attempts: 2}

	ingestions := &fakeIngestionService{pending: []*model.Ingestion{ing}}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{}}
	processor := &fakeProcessor{}
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0, 3)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if ing.Status != model.IngestionFailed {
		t.Errorf("expected status failed with attempts exhausted, got %s", ing.Status)
	}
	if processor.processedCount() != 0 {
		t.Errorf("expected processor not to be called, got %d", processor.processedCount())
	}
	if chunkStore.savedCount() != 0 {
		t.Errorf("expected chunk store not to be called, got %d calls", chunkStore.savedCount())
	}
}

func TestWorker_Run_NoPendingIngestions_NoOp(t *testing.T) {
	ingestions := &fakeIngestionService{pending: nil}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{}}
	processor := &fakeProcessor{}
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0, 0)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if len(ingestions.calls) != 0 {
		t.Errorf("expected no UpdateStatus calls, got %d", len(ingestions.calls))
	}
	if processor.processedCount() != 0 {
		t.Errorf("expected processor not to be called, got %d", processor.processedCount())
	}
	if chunkStore.savedCount() != 0 {
		t.Errorf("expected chunk store not to be called, got %d calls", chunkStore.savedCount())
	}
}

func TestWorker_Run_ClaimError_ReturnsError(t *testing.T) {
	claimErr := errors.New("database unavailable")

	ingestions := &fakeIngestionService{listErr: claimErr}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{}}
	processor := &fakeProcessor{}
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0, 0)

	err := w.Run(context.Background())
	if !errors.Is(err, claimErr) {
		t.Fatalf("expected error %v, got %v", claimErr, err)
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

	w := New(ingestions, documents, processor, embedder, chunkStore, 2, 0)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if processor.processedCount() != 2 {
		t.Errorf("expected 2 documents processed with batch size 2, got %d", processor.processedCount())
	}
}

func TestWorker_New_DefaultsBatchSizeAndMaxAttemptsWhenZeroOrNegative(t *testing.T) {
	ingestions := &fakeIngestionService{}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{}}
	processor := &fakeProcessor{}
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0, 0)
	if w.batchSize != defaultBatchSize {
		t.Errorf("expected default batch size %d, got %d", defaultBatchSize, w.batchSize)
	}
	if w.maxAttempts != defaultMaxAttempts {
		t.Errorf("expected default max attempts %d, got %d", defaultMaxAttempts, w.maxAttempts)
	}

	w = New(ingestions, documents, processor, embedder, chunkStore, -5, -1)
	if w.batchSize != defaultBatchSize {
		t.Errorf("expected default batch size %d for negative input, got %d", defaultBatchSize, w.batchSize)
	}
	if w.maxAttempts != defaultMaxAttempts {
		t.Errorf("expected default max attempts %d for negative input, got %d", defaultMaxAttempts, w.maxAttempts)
	}
}

// --- RunLoop tests -------------------------------------------------------

func TestWorker_RunLoop_PollsRepeatedlyUntilCancelled(t *testing.T) {
	ingestions := &fakeIngestionService{}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{}}
	processor := &fakeProcessor{}
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0, 0)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- w.RunLoop(ctx, 5*time.Millisecond, nil)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

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

	w := New(ingestions, documents, processor, embedder, chunkStore, 0, 0)

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

	w := New(ingestions, documents, processor, embedder, chunkStore, 0, 0)

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
	if ingestions.listCallCount < 2 {
		t.Errorf("expected RunLoop to keep polling despite errors, got %d calls", ingestions.listCallCount)
	}
}

func TestWorker_RunLoop_DefaultsIntervalWhenZero(t *testing.T) {
	ingestions := &fakeIngestionService{}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{}}
	processor := &fakeProcessor{}
	embedder := &fakeEmbedder{}
	chunkStore := &fakeChunkStore{}

	w := New(ingestions, documents, processor, embedder, chunkStore, 0, 0)

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

// --- Processor test double (unchanged from earlier phases) -------------

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

// conditionalFailNTimesProcessor fails its first failTimes calls, then
// succeeds — used to simulate a transient failure that clears up on
// retry, so a test can verify a requeued ingestion is genuinely
// re-claimed and re-processed on a later pass rather than lost.
type conditionalFailNTimesProcessor struct {
	failTimes int
	chunksFor map[string][]string

	mu    sync.Mutex
	calls int
}

func (p *conditionalFailNTimesProcessor) Process(ctx context.Context, doc *model.Document) ([]string, error) {
	p.mu.Lock()
	p.calls++
	callNum := p.calls
	p.mu.Unlock()

	if callNum <= p.failTimes {
		return nil, errors.New("simulated transient failure")
	}
	return p.chunksFor[doc.ID], nil
}
