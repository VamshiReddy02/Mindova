package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/vamshireddy02/mindova/services/document-service/embedding"
	"github.com/vamshireddy02/mindova/services/document-service/model"
	"github.com/vamshireddy02/mindova/services/document-service/repository"
)

// defaultRetrievalLimit and maxRetrievalLimit bound the top-K a caller may
// request. Mirrors the same shape of guard used for document listing, but
// enforced strictly here (an out-of-range limit is an error) rather than
// silently clamped, since Search has no HTTP layer in front of it yet to
// apply its own defaulting.
const (
	minRetrievalLimit = 1
	maxRetrievalLimit = 50
)

// ErrEmptyQuery is returned when Search is called with an empty (or
// whitespace-only) query string.
var ErrEmptyQuery = errors.New("retrieval: query must not be empty")

// ErrInvalidLimit is returned when the requested limit is out of range.
var ErrInvalidLimit = errors.New("retrieval: limit is invalid")

// RetrievalService turns a natural-language query into the most relevant
// document chunks, by embedding the query and running a vector similarity
// search over previously ingested chunks.
type RetrievalService interface {
	Search(ctx context.Context, query string, limit int) ([]*model.DocumentChunk, error)
}

type retrievalService struct {
	embedder embedding.Embedder
	chunks   repository.ChunkRepository
}

// NewRetrievalService creates a RetrievalService backed by the given
// Embedder (to turn the query into a vector) and ChunkRepository (to
// search for chunks near that vector).
func NewRetrievalService(embedder embedding.Embedder, chunks repository.ChunkRepository) RetrievalService {
	return &retrievalService{
		embedder: embedder,
		chunks:   chunks,
	}
}

// Search embeds query and returns the limit most similar document chunks,
// nearest first.
//
// Flow:
//
//	query text -> validate -> embed -> SearchSimilar -> top-K chunks
//
// Returns ErrEmptyQuery if query is empty/whitespace-only, ErrInvalidLimit
// if limit is out of [1, maxRetrievalLimit], and otherwise propagates any
// error from the embedder or the chunk repository, wrapped with context
// about which stage failed.
func (s *retrievalService) Search(ctx context.Context, query string, limit int) ([]*model.DocumentChunk, error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil, ErrEmptyQuery
	}

	if limit < minRetrievalLimit || limit > maxRetrievalLimit {
		return nil, fmt.Errorf(
			"%w: got %d, must be between %d and %d",
			ErrInvalidLimit, limit, minRetrievalLimit, maxRetrievalLimit,
		)
	}

	embeddings, err := s.embedder.Embed(ctx, []string{trimmed})
	if err != nil {
		return nil, fmt.Errorf("retrieval: failed to embed query: %w", err)
	}
	if len(embeddings) == 0 {
		return nil, errors.New("retrieval: embedder returned no vectors for query")
	}

	chunks, err := s.chunks.SearchSimilar(ctx, embeddings[0], limit)
	if err != nil {
		return nil, fmt.Errorf("retrieval: search failed: %w", err)
	}

	return chunks, nil
}

// Compile-time check that *retrievalService satisfies RetrievalService.
var _ RetrievalService = (*retrievalService)(nil)
