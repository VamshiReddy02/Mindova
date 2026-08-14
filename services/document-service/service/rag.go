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

const ragInstruction = "Answer the question using ONLY the retrieved context below. " +
	"If the answer cannot be found in the context, say you don't know — do not guess."

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
		{Role: "system", Content: buildSystemPrompt(chunks)},
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

func buildSystemPrompt(chunks []*model.DocumentChunk) string {
	var b strings.Builder

	b.WriteString(ragInstruction)
	b.WriteString("\n\n")
	b.WriteString("Retrieved context:\n")

	if len(chunks) == 0 {
		b.WriteString("No relevant context was found.")
		return b.String()
	}

	for i, c := range chunks {
		fmt.Fprintf(&b, "[Chunk %d]\n%s", i+1, c.Content)
		if i < len(chunks)-1 {
			b.WriteString("\n\n")
		}
	}

	return b.String()
}

// Compile-time check that *ragService satisfies RAGService.
var _ RAGService = (*ragService)(nil)
