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
	pending   []*model.Ingestion
	listErr   error
	updateErr error

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
	return f.updateErr
}

// fakeDocumentGetter is an in-memory stand-in for worker.DocumentGetter.
type fakeDocumentGetter struct {
	docs map[string]*model.Document
	err  error
}

func (f *fakeDocumentGetter) GetByID(ctx context.Context, id string) (*model.Document, error) {
	if f.err != nil {
		return nil, f.err
	}
	doc, ok := f.docs[id]
	if !ok {
		return nil, errors.New("document not found")
	}
	return doc, nil
}

// fakeProcessor is an in-memory stand-in for worker.Processor.
type fakeProcessor struct {
	err       error
	processed []string // document IDs passed to Process, in order
}

func (f *fakeProcessor) Process(ctx context.Context, doc *model.Document) error {
	f.processed = append(f.processed, doc.ID)
	return f.err
}

func TestWorker_Run_ProcessesPendingIngestion_Success(t *testing.T) {
	ing := &model.Ingestion{ID: "ing-1", DocumentID: "doc-1", Status: model.IngestionPending}
	doc := &model.Document{ID: "doc-1", Name: "a.md"}

	ingestions := &fakeIngestionService{pending: []*model.Ingestion{ing}}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{"doc-1": doc}}
	processor := &fakeProcessor{}

	w := New(ingestions, documents, processor, 0)

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

	if len(processor.processed) != 1 || processor.processed[0] != "doc-1" {
		t.Errorf("expected processor to be called with doc-1, got %v", processor.processed)
	}
}

func TestWorker_Run_ProcessingFails_MarksFailed(t *testing.T) {
	ing := &model.Ingestion{ID: "ing-1", DocumentID: "doc-1", Status: model.IngestionPending}
	doc := &model.Document{ID: "doc-1", Name: "a.md"}
	processErr := errors.New("embedding service unavailable")

	ingestions := &fakeIngestionService{pending: []*model.Ingestion{ing}}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{"doc-1": doc}}
	processor := &fakeProcessor{err: processErr}

	w := New(ingestions, documents, processor, 0)

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
	if final.errMsg != processErr.Error() {
		t.Errorf("expected error message %q, got %q", processErr.Error(), final.errMsg)
	}
}

func TestWorker_Run_DocumentFetchFails_MarksFailed(t *testing.T) {
	ing := &model.Ingestion{ID: "ing-1", DocumentID: "doc-missing", Status: model.IngestionPending}

	ingestions := &fakeIngestionService{pending: []*model.Ingestion{ing}}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{}} // empty: doc-missing isn't there
	processor := &fakeProcessor{}

	w := New(ingestions, documents, processor, 0)

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

	// The processor should never have been reached since the document
	// couldn't be loaded.
	if len(processor.processed) != 0 {
		t.Errorf("expected processor not to be called, got %v", processor.processed)
	}
}

func TestWorker_Run_NoPendingIngestions_NoOp(t *testing.T) {
	ingestions := &fakeIngestionService{pending: nil}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{}}
	processor := &fakeProcessor{}

	w := New(ingestions, documents, processor, 0)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if len(ingestions.calls) != 0 {
		t.Errorf("expected no UpdateStatus calls, got %d", len(ingestions.calls))
	}
	if len(processor.processed) != 0 {
		t.Errorf("expected processor not to be called, got %v", processor.processed)
	}
}

func TestWorker_Run_ListPendingError_ReturnsError(t *testing.T) {
	listErr := errors.New("database unavailable")

	ingestions := &fakeIngestionService{listErr: listErr}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{}}
	processor := &fakeProcessor{}

	w := New(ingestions, documents, processor, 0)

	err := w.Run(context.Background())
	if !errors.Is(err, listErr) {
		t.Fatalf("expected error %v, got %v", listErr, err)
	}

	if len(ingestions.calls) != 0 {
		t.Errorf("expected no UpdateStatus calls when listing pending fails, got %d", len(ingestions.calls))
	}
	if len(processor.processed) != 0 {
		t.Errorf("expected processor not to be called, got %v", processor.processed)
	}
}

func TestWorker_Run_MultipleIngestions_OneFailureDoesNotStopBatch(t *testing.T) {
	okIng := &model.Ingestion{ID: "ing-ok", DocumentID: "doc-ok", Status: model.IngestionPending}
	badIng := &model.Ingestion{ID: "ing-bad", DocumentID: "doc-bad", Status: model.IngestionPending}

	docOK := &model.Document{ID: "doc-ok", Name: "ok.md"}
	docBad := &model.Document{ID: "doc-bad", Name: "bad.md"}

	ingestions := &fakeIngestionService{pending: []*model.Ingestion{badIng, okIng}}
	documents := &fakeDocumentGetter{docs: map[string]*model.Document{
		"doc-ok":  docOK,
		"doc-bad": docBad,
	}}

	processErr := errors.New("processing failed")
	processor := &conditionalProcessor{
		failFor: map[string]error{"doc-bad": processErr},
	}

	w := New(ingestions, documents, processor, 0)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	// 2 calls per ingestion (processing + terminal) x 2 ingestions = 4.
	if len(ingestions.calls) != 4 {
		t.Fatalf("expected 4 UpdateStatus calls, got %d: %+v", len(ingestions.calls), ingestions.calls)
	}

	var gotBadFinal, gotOKFinal *statusCall
	for i := range ingestions.calls {
		c := ingestions.calls[i]
		if c.id == "ing-bad" && (c.status == model.IngestionFailed || c.status == model.IngestionCompleted) {
			gotBadFinal = &ingestions.calls[i]
		}
		if c.id == "ing-ok" && (c.status == model.IngestionFailed || c.status == model.IngestionCompleted) {
			gotOKFinal = &ingestions.calls[i]
		}
	}

	if gotBadFinal == nil || gotBadFinal.status != model.IngestionFailed {
		t.Errorf("expected ing-bad to end failed, got %+v", gotBadFinal)
	}
	if gotOKFinal == nil || gotOKFinal.status != model.IngestionCompleted {
		t.Errorf("expected ing-ok to end completed despite ing-bad failing, got %+v", gotOKFinal)
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

	w := New(ingestions, documents, processor, 2)

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

	w := New(ingestions, documents, processor, 0)
	if w.batchSize != defaultBatchSize {
		t.Errorf("expected default batch size %d, got %d", defaultBatchSize, w.batchSize)
	}

	w = New(ingestions, documents, processor, -5)
	if w.batchSize != defaultBatchSize {
		t.Errorf("expected default batch size %d for negative input, got %d", defaultBatchSize, w.batchSize)
	}
}

// conditionalProcessor fails only for specific document IDs, letting a
// test simulate one bad document among several good ones.
type conditionalProcessor struct {
	failFor map[string]error
}

func (p *conditionalProcessor) Process(ctx context.Context, doc *model.Document) error {
	if err, ok := p.failFor[doc.ID]; ok {
		return err
	}
	return nil
}
