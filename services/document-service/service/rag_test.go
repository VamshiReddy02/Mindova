package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vamshireddy02/mindova/services/document-service/llm"
	"github.com/vamshireddy02/mindova/services/document-service/model"
)

// fakeRetrievalService is an in-memory stand-in for RetrievalService.
type fakeRetrievalService struct {
	chunks []*model.DocumentChunk
	err    error

	// recorded call args
	gotQuery string
	gotLimit int
}

func (f *fakeRetrievalService) Search(ctx context.Context, query string, limit int) ([]*model.DocumentChunk, error) {
	f.gotQuery = query
	f.gotLimit = limit
	if f.err != nil {
		return nil, f.err
	}
	return f.chunks, nil
}

// fakeLLMClient is an in-memory stand-in for llm.Client.
type fakeLLMClient struct {
	answer string
	err    error

	// recorded call args
	gotMessages []llm.Message
	callCount   int
}

func (f *fakeLLMClient) Complete(ctx context.Context, messages []llm.Message) (string, error) {
	f.callCount++
	f.gotMessages = messages
	if f.err != nil {
		return "", f.err
	}
	return f.answer, nil
}

func TestRAGService_Ask_Success(t *testing.T) {
	chunks := []*model.DocumentChunk{
		{ID: "chunk-1", Content: "Mindova chunks documents before embedding them."},
		{ID: "chunk-2", Content: "Embeddings are stored in pgvector."},
	}

	retrieval := &fakeRetrievalService{chunks: chunks}
	llmClient := &fakeLLMClient{answer: "Mindova chunks documents, embeds them, and stores vectors in pgvector."}

	svc := NewRAGService(retrieval, llmClient)

	resp, err := svc.Ask(context.Background(), "How does Mindova process documents?", 5)
	if err != nil {
		t.Fatalf("Ask() returned error: %v", err)
	}

	if resp.Answer != llmClient.answer {
		t.Errorf("expected answer %q, got %q", llmClient.answer, resp.Answer)
	}
	if len(resp.Chunks) != 2 {
		t.Fatalf("expected 2 source chunks, got %d", len(resp.Chunks))
	}
	if resp.Chunks[0].ID != "chunk-1" || resp.Chunks[1].ID != "chunk-2" {
		t.Errorf("unexpected chunks in response: %+v", resp.Chunks)
	}
}

func TestRAGService_Ask_EmptyQuestion_ReturnsError(t *testing.T) {
	svc := NewRAGService(&fakeRetrievalService{}, &fakeLLMClient{})

	_, err := svc.Ask(context.Background(), "", 5)
	if !errors.Is(err, ErrEmptyQuery) {
		t.Fatalf("expected ErrEmptyQuery, got %v", err)
	}
}

func TestRAGService_Ask_WhitespaceOnlyQuestion_ReturnsError(t *testing.T) {
	svc := NewRAGService(&fakeRetrievalService{}, &fakeLLMClient{})

	_, err := svc.Ask(context.Background(), "   \t\n  ", 5)
	if !errors.Is(err, ErrEmptyQuery) {
		t.Fatalf("expected ErrEmptyQuery, got %v", err)
	}
}

func TestRAGService_Ask_RetrievalError_LLMNeverCalled(t *testing.T) {
	retrievalErr := errors.New("embedding service unavailable")
	retrieval := &fakeRetrievalService{err: retrievalErr}
	llmClient := &fakeLLMClient{}

	svc := NewRAGService(retrieval, llmClient)

	_, err := svc.Ask(context.Background(), "a question", 5)
	if !errors.Is(err, retrievalErr) {
		t.Fatalf("expected error to wrap %v, got %v", retrievalErr, err)
	}

	if llmClient.callCount != 0 {
		t.Errorf("expected llm.Complete never called when retrieval fails, got %d calls", llmClient.callCount)
	}
}

func TestRAGService_Ask_GenerationError_Propagates(t *testing.T) {
	genErr := errors.New("llm provider unavailable")
	retrieval := &fakeRetrievalService{chunks: []*model.DocumentChunk{{ID: "c1", Content: "some content"}}}
	llmClient := &fakeLLMClient{err: genErr}

	svc := NewRAGService(retrieval, llmClient)

	_, err := svc.Ask(context.Background(), "a question", 5)
	if !errors.Is(err, genErr) {
		t.Fatalf("expected error to wrap %v, got %v", genErr, err)
	}
}

func TestRAGService_Ask_NoChunksFound_StillCallsLLM(t *testing.T) {
	retrieval := &fakeRetrievalService{chunks: []*model.DocumentChunk{}}
	llmClient := &fakeLLMClient{answer: "I don't have enough information to answer that."}

	svc := NewRAGService(retrieval, llmClient)

	resp, err := svc.Ask(context.Background(), "an unanswerable question", 5)
	if err != nil {
		t.Fatalf("Ask() returned error: %v", err)
	}

	if llmClient.callCount != 1 {
		t.Fatalf("expected llm.Complete to still be called with zero chunks, got %d calls", llmClient.callCount)
	}
	if len(resp.Chunks) != 0 {
		t.Errorf("expected zero source chunks, got %d", len(resp.Chunks))
	}

	systemMessage := llmClient.gotMessages[0]
	if systemMessage.Role != "system" {
		t.Fatalf("expected first message to be system role, got %s", systemMessage.Role)
	}
	if systemMessage.Content != "No relevant context was found." {
		t.Errorf("expected explicit no-context message, got %q", systemMessage.Content)
	}
}

func TestRAGService_Ask_BuildsMessagesCorrectly(t *testing.T) {
	chunks := []*model.DocumentChunk{
		{Content: "first chunk content"},
		{Content: "second chunk content"},
	}
	retrieval := &fakeRetrievalService{chunks: chunks}
	llmClient := &fakeLLMClient{answer: "an answer"}

	svc := NewRAGService(retrieval, llmClient)

	if _, err := svc.Ask(context.Background(), "  the question  ", 5); err != nil {
		t.Fatalf("Ask() returned error: %v", err)
	}

	if len(llmClient.gotMessages) != 2 {
		t.Fatalf("expected 2 messages sent to llm.Complete, got %d", len(llmClient.gotMessages))
	}

	systemMsg := llmClient.gotMessages[0]
	userMsg := llmClient.gotMessages[1]

	if systemMsg.Role != "system" {
		t.Errorf("expected first message role system, got %s", systemMsg.Role)
	}
	if userMsg.Role != "user" {
		t.Errorf("expected second message role user, got %s", userMsg.Role)
	}
	if userMsg.Content != "the question" {
		t.Errorf("expected trimmed question %q, got %q", "the question", userMsg.Content)
	}

	if !strings.Contains(systemMsg.Content, "first chunk content") || !strings.Contains(systemMsg.Content, "second chunk content") {
		t.Errorf("expected system message to contain both chunks' content, got %q", systemMsg.Content)
	}
	if !strings.Contains(systemMsg.Content, "[1]") || !strings.Contains(systemMsg.Content, "[2]") {
		t.Errorf("expected system message to number chunks [1] and [2], got %q", systemMsg.Content)
	}
}

func TestRAGService_Ask_TrimsQuestionBeforeRetrieval(t *testing.T) {
	retrieval := &fakeRetrievalService{chunks: nil}
	llmClient := &fakeLLMClient{answer: "answer"}

	svc := NewRAGService(retrieval, llmClient)

	if _, err := svc.Ask(context.Background(), "  spaced question  ", 5); err != nil {
		t.Fatalf("Ask() returned error: %v", err)
	}

	if retrieval.gotQuery != "spaced question" {
		t.Errorf("expected retrieval called with trimmed query %q, got %q", "spaced question", retrieval.gotQuery)
	}
}

func TestRAGService_Ask_PassesLimitThroughToRetrieval(t *testing.T) {
	retrieval := &fakeRetrievalService{chunks: nil}
	llmClient := &fakeLLMClient{answer: "answer"}

	svc := NewRAGService(retrieval, llmClient)

	if _, err := svc.Ask(context.Background(), "a question", 7); err != nil {
		t.Fatalf("Ask() returned error: %v", err)
	}

	if retrieval.gotLimit != 7 {
		t.Errorf("expected retrieval called with limit 7, got %d", retrieval.gotLimit)
	}
}
