package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/vamshireddy02/mindova/services/document-service/llm"
	"github.com/vamshireddy02/mindova/services/document-service/model"
)

type RAGResponse struct {
	Answer string
	Chunks []*model.DocumentChunk
}

type RAGService interface {
	Ask(ctx context.Context, question string, limit int) (*RAGResponse, error)
}

type ragService struct {
	retrieval RetrievalService
	llm       llm.Client
}

func NewRAGService(retrieval RetrievalService, llmClient llm.Client) RAGService {
	return &ragService{
		retrieval: retrieval,
		llm:       llmClient,
	}
}

func (s *ragService) Ask(ctx context.Context, question string, limit int) (*RAGResponse, error) {
	trimmed := strings.TrimSpace(question)
	if trimmed == "" {
		return nil, ErrEmptyQuery
	}

	chunks, err := s.retrieval.Search(ctx, trimmed, limit)
	if err != nil {
		return nil, fmt.Errorf("rag: retrieval failed: %w", err)
	}

	messages := []llm.Message{
		{Role: "system", Content: buildContext(chunks)},
		{Role: "user", Content: trimmed},
	}

	answer, err := s.llm.Complete(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("rag: generation failed: %w", err)
	}

	return &RAGResponse{
		Answer: answer,
		Chunks: chunks,
	}, nil
}

func buildContext(chunks []*model.DocumentChunk) string {
	if len(chunks) == 0 {
		return "No relevant context was found."
	}

	var b strings.Builder
	b.WriteString("Context:\n")
	for i, c := range chunks {
		fmt.Fprintf(&b, "[%d] %s\n", i+1, c.Content)
	}
	return b.String()
}

// Compile-time check that *ragService satisfies RAGService.
var _ RAGService = (*ragService)(nil)
