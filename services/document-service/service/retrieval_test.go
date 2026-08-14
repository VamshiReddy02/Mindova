package service

import (
	"context"
	"errors"
	"testing"

	"github.com/vamshireddy02/mindova/services/document-service/model"
)

// fakeEmbedder is an in-memory stand-in for embedding.Embedder.
type fakeEmbedder struct {
	vectors [][]float32 // returned as-is
	err     error

	calledWith [][]string // each call's input texts, in order
}

func (f *fakeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	f.calledWith = append(f.calledWith, texts)
	if f.err != nil {
		return nil, f.err
	}
	return f.vectors, nil
}

// mockChunkRepository is an in-memory stand-in for repository.ChunkRepository.
// Only searchSimilarFn is used by these tests; CreateBatch and
// GetByDocumentID are unused here but must be implemented to satisfy the
// interface, so they return errUnimplemented if ever called by mistake.
type mockChunkRepository struct {
	searchSimilarFn func(ctx context.Context, embedding []float32, limit int) ([]*model.DocumentChunk, error)
}

func (m *mockChunkRepository) CreateBatch(ctx context.Context, chunks []*model.DocumentChunk) error {
	return errUnimplemented
}

func (m *mockChunkRepository) GetByDocumentID(ctx context.Context, documentID string) ([]*model.DocumentChunk, error) {
	return nil, errUnimplemented
}

func (m *mockChunkRepository) SearchSimilar(ctx context.Context, embedding []float32, limit int) ([]*model.DocumentChunk, error) {
	if m.searchSimilarFn == nil {
		return nil, errUnimplemented
	}
	return m.searchSimilarFn(ctx, embedding, limit)
}

func TestRetrievalService_Search_Success(t *testing.T) {
	queryVector := []float32{1, 0, 0, 0, 0, 0, 0, 0}
	want := []*model.DocumentChunk{
		{ID: "chunk-1", Content: "most relevant"},
		{ID: "chunk-2", Content: "second most relevant"},
	}

	var gotEmbedding []float32
	var gotLimit int

	embedder := &fakeEmbedder{vectors: [][]float32{queryVector}}
	chunks := &mockChunkRepository{
		searchSimilarFn: func(ctx context.Context, embedding []float32, limit int) ([]*model.DocumentChunk, error) {
			gotEmbedding = embedding
			gotLimit = limit
			return want, nil
		},
	}

	svc := NewRetrievalService(embedder, chunks)

	got, err := svc.Search(context.Background(), "what is mindova?", 5)
	if err != nil {
		t.Fatalf("Search() returned error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(got))
	}
	if got[0].ID != "chunk-1" || got[1].ID != "chunk-2" {
		t.Errorf("unexpected chunks returned: %+v", got)
	}

	if len(gotEmbedding) != len(queryVector) {
		t.Fatalf("expected embedding of length %d passed to SearchSimilar, got %d", len(queryVector), len(gotEmbedding))
	}
	if gotLimit != 5 {
		t.Errorf("expected limit 5 passed to SearchSimilar, got %d", gotLimit)
	}
}

func TestRetrievalService_Search_EmbedsTheQueryText(t *testing.T) {
	embedder := &fakeEmbedder{vectors: [][]float32{{1, 0, 0, 0, 0, 0, 0, 0}}}
	chunks := &mockChunkRepository{
		searchSimilarFn: func(ctx context.Context, embedding []float32, limit int) ([]*model.DocumentChunk, error) {
			return nil, nil
		},
	}

	svc := NewRetrievalService(embedder, chunks)

	if _, err := svc.Search(context.Background(), "  what is mindova?  ", 5); err != nil {
		t.Fatalf("Search() returned error: %v", err)
	}

	if len(embedder.calledWith) != 1 {
		t.Fatalf("expected Embed to be called once, got %d calls", len(embedder.calledWith))
	}

	texts := embedder.calledWith[0]
	if len(texts) != 1 {
		t.Fatalf("expected exactly 1 text passed to Embed, got %d", len(texts))
	}
	if texts[0] != "what is mindova?" {
		t.Errorf("expected trimmed query passed to Embed, got %q", texts[0])
	}
}

func TestRetrievalService_Search_EmptyQuery_ReturnsError(t *testing.T) {
	svc := NewRetrievalService(&fakeEmbedder{}, &mockChunkRepository{})

	_, err := svc.Search(context.Background(), "", 5)
	if !errors.Is(err, ErrEmptyQuery) {
		t.Fatalf("expected ErrEmptyQuery, got %v", err)
	}
}

func TestRetrievalService_Search_WhitespaceOnlyQuery_ReturnsError(t *testing.T) {
	svc := NewRetrievalService(&fakeEmbedder{}, &mockChunkRepository{})

	_, err := svc.Search(context.Background(), "   \t  ", 5)
	if !errors.Is(err, ErrEmptyQuery) {
		t.Fatalf("expected ErrEmptyQuery, got %v", err)
	}
}

func TestRetrievalService_Search_InvalidLimit(t *testing.T) {
	tests := []struct {
		name  string
		limit int
	}{
		{"zero", 0},
		{"negative", -1},
		{"exceeds maximum", maxRetrievalLimit + 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewRetrievalService(&fakeEmbedder{}, &mockChunkRepository{})

			_, err := svc.Search(context.Background(), "a valid query", tt.limit)
			if !errors.Is(err, ErrInvalidLimit) {
				t.Fatalf("expected ErrInvalidLimit for limit=%d, got %v", tt.limit, err)
			}
		})
	}
}

func TestRetrievalService_Search_ValidLimitBoundaries(t *testing.T) {
	embedder := &fakeEmbedder{vectors: [][]float32{{1, 0, 0, 0, 0, 0, 0, 0}}}
	chunks := &mockChunkRepository{
		searchSimilarFn: func(ctx context.Context, embedding []float32, limit int) ([]*model.DocumentChunk, error) {
			return nil, nil
		},
	}
	svc := NewRetrievalService(embedder, chunks)

	if _, err := svc.Search(context.Background(), "query", minRetrievalLimit); err != nil {
		t.Errorf("expected minRetrievalLimit (%d) to be valid, got error: %v", minRetrievalLimit, err)
	}
	if _, err := svc.Search(context.Background(), "query", maxRetrievalLimit); err != nil {
		t.Errorf("expected maxRetrievalLimit (%d) to be valid, got error: %v", maxRetrievalLimit, err)
	}
}

func TestRetrievalService_Search_EmbeddingError_Propagates(t *testing.T) {
	embedErr := errors.New("embedding provider unavailable")
	embedder := &fakeEmbedder{err: embedErr}
	chunks := &mockChunkRepository{
		searchSimilarFn: func(ctx context.Context, embedding []float32, limit int) ([]*model.DocumentChunk, error) {
			t.Fatal("SearchSimilar should not be called when embedding fails")
			return nil, nil
		},
	}

	svc := NewRetrievalService(embedder, chunks)

	_, err := svc.Search(context.Background(), "a query", 5)
	if !errors.Is(err, embedErr) {
		t.Fatalf("expected error to wrap %v, got %v", embedErr, err)
	}
}

func TestRetrievalService_Search_SearchSimilarError_Propagates(t *testing.T) {
	searchErr := errors.New("database unavailable")
	embedder := &fakeEmbedder{vectors: [][]float32{{1, 0, 0, 0, 0, 0, 0, 0}}}
	chunks := &mockChunkRepository{
		searchSimilarFn: func(ctx context.Context, embedding []float32, limit int) ([]*model.DocumentChunk, error) {
			return nil, searchErr
		},
	}

	svc := NewRetrievalService(embedder, chunks)

	_, err := svc.Search(context.Background(), "a query", 5)
	if !errors.Is(err, searchErr) {
		t.Fatalf("expected error to wrap %v, got %v", searchErr, err)
	}
}

func TestRetrievalService_Search_NoVectorsReturned_ReturnsError(t *testing.T) {
	// Defensive case: an embedder that returns success but zero vectors
	// for a single input text would be a bug in the embedder, but Search
	// should fail loudly rather than pass a nil/empty embedding on to
	// SearchSimilar.
	embedder := &fakeEmbedder{vectors: [][]float32{}}
	chunks := &mockChunkRepository{
		searchSimilarFn: func(ctx context.Context, embedding []float32, limit int) ([]*model.DocumentChunk, error) {
			t.Fatal("SearchSimilar should not be called when no vectors are returned")
			return nil, nil
		},
	}

	svc := NewRetrievalService(embedder, chunks)

	_, err := svc.Search(context.Background(), "a query", 5)
	if err == nil {
		t.Fatal("expected an error when the embedder returns zero vectors, got nil")
	}
}
