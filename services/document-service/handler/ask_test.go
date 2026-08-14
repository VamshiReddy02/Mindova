package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vamshireddy02/mindova/services/document-service/model"
	"github.com/vamshireddy02/mindova/services/document-service/service"
)

func TestAsk_Success(t *testing.T) {
	want := &service.RAGResponse{
		Answer: "Mindova chunks documents, embeds them, and stores vectors in pgvector.",
		Chunks: []*model.DocumentChunk{
			{DocumentID: "doc-1", ChunkIndex: 0, Content: "Mindova chunks documents before embedding them."},
			{DocumentID: "doc-1", ChunkIndex: 1, Content: "Embeddings are stored in pgvector."},
		},
	}

	var gotQuestion string
	var gotLimit int

	rag := &stubRAGService{
		askFn: func(ctx context.Context, question string, limit int) (*service.RAGResponse, error) {
			gotQuestion = question
			gotLimit = limit
			return want, nil
		},
	}
	h := New(&stubService{}, &stubRetrievalService{}, rag)

	body := `{"question":"How does Mindova process documents?","limit":5}`
	req := httptest.NewRequest(http.MethodPost, "/documents/ask", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Ask(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if gotQuestion != "How does Mindova process documents?" {
		t.Errorf("expected question passed through, got %q", gotQuestion)
	}
	if gotLimit != 5 {
		t.Errorf("expected limit 5 passed through, got %d", gotLimit)
	}

	var got askResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if got.Answer != want.Answer {
		t.Errorf("expected answer %q, got %q", want.Answer, got.Answer)
	}
	if len(got.Sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(got.Sources))
	}
	if got.Sources[0].DocumentID != "doc-1" || got.Sources[0].ChunkIndex != 0 {
		t.Errorf("unexpected first source: %+v", got.Sources[0])
	}
	if got.Sources[1].ChunkIndex != 1 {
		t.Errorf("unexpected second source: %+v", got.Sources[1])
	}
}

func TestAsk_ResponseShape_OnlyExpectedFields(t *testing.T) {
	rag := &stubRAGService{
		askFn: func(ctx context.Context, question string, limit int) (*service.RAGResponse, error) {
			return &service.RAGResponse{
				Answer: "an answer",
				Chunks: []*model.DocumentChunk{
					{ID: "chunk-1", DocumentID: "doc-1", ChunkIndex: 0, Content: "content"},
				},
			}, nil
		},
	}
	h := New(&stubService{}, &stubRetrievalService{}, rag)

	body := `{"question":"a question","limit":5}`
	req := httptest.NewRequest(http.MethodPost, "/documents/ask", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Ask(rec, req)

	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if _, ok := raw["answer"]; !ok {
		t.Error("expected top-level \"answer\" field")
	}
	sources, ok := raw["sources"].([]any)
	if !ok {
		t.Fatal("expected top-level \"sources\" array")
	}
	if len(sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(sources))
	}

	source, ok := sources[0].(map[string]any)
	if !ok {
		t.Fatal("expected source to be an object")
	}

	wantKeys := map[string]bool{"document_id": true, "chunk_index": true, "content": true}
	for key := range source {
		if !wantKeys[key] {
			t.Errorf("unexpected key %q in source (chunk's internal ID/embedding should not leak)", key)
		}
	}
	if _, ok := source["document_id"]; !ok {
		t.Error("expected \"document_id\" in source")
	}
}

func TestAsk_EmptyQuestion(t *testing.T) {
	rag := &stubRAGService{
		askFn: func(ctx context.Context, question string, limit int) (*service.RAGResponse, error) {
			return nil, service.ErrEmptyQuery
		},
	}
	h := New(&stubService{}, &stubRetrievalService{}, rag)

	body := `{"question":"","limit":5}`
	req := httptest.NewRequest(http.MethodPost, "/documents/ask", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Ask(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestAsk_WhitespaceQuestion(t *testing.T) {
	rag := &stubRAGService{
		askFn: func(ctx context.Context, question string, limit int) (*service.RAGResponse, error) {
			return nil, service.ErrEmptyQuery
		},
	}
	h := New(&stubService{}, &stubRetrievalService{}, rag)

	body := `{"question":"   \t\n  ","limit":5}`
	req := httptest.NewRequest(http.MethodPost, "/documents/ask", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Ask(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestAsk_InvalidLimit(t *testing.T) {
	rag := &stubRAGService{
		askFn: func(ctx context.Context, question string, limit int) (*service.RAGResponse, error) {
			return nil, service.ErrInvalidLimit
		},
	}
	h := New(&stubService{}, &stubRetrievalService{}, rag)

	body := `{"question":"a valid question","limit":0}`
	req := httptest.NewRequest(http.MethodPost, "/documents/ask", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Ask(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestAsk_ServiceError(t *testing.T) {
	rag := &stubRAGService{
		askFn: func(ctx context.Context, question string, limit int) (*service.RAGResponse, error) {
			return nil, errors.New("llm provider unavailable")
		},
	}
	h := New(&stubService{}, &stubRetrievalService{}, rag)

	body := `{"question":"a valid question","limit":5}`
	req := httptest.NewRequest(http.MethodPost, "/documents/ask", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Ask(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
}

func TestAsk_RetrievalError(t *testing.T) {
	rag := &stubRAGService{
		askFn: func(ctx context.Context, question string, limit int) (*service.RAGResponse, error) {
			// Mirrors RAGService.Ask wrapping a retrieval-stage failure.
			return nil, fmt.Errorf("rag: retrieval failed: %w", errors.New("embedding service unreachable"))
		},
	}
	h := New(&stubService{}, &stubRetrievalService{}, rag)

	body := `{"question":"a valid question","limit":5}`
	req := httptest.NewRequest(http.MethodPost, "/documents/ask", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Ask(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
}

func TestAsk_LLMError(t *testing.T) {
	rag := &stubRAGService{
		askFn: func(ctx context.Context, question string, limit int) (*service.RAGResponse, error) {
			// Mirrors RAGService.Ask wrapping a generation-stage failure.
			return nil, fmt.Errorf("rag: generation failed: %w", errors.New("llm provider timed out"))
		},
	}
	h := New(&stubService{}, &stubRetrievalService{}, rag)

	body := `{"question":"a valid question","limit":5}`
	req := httptest.NewRequest(http.MethodPost, "/documents/ask", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Ask(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
}

func TestAsk_MalformedJSON(t *testing.T) {
	rag := &stubRAGService{
		askFn: func(ctx context.Context, question string, limit int) (*service.RAGResponse, error) {
			t.Fatal("service should not be called for malformed JSON")
			return nil, nil
		},
	}
	h := New(&stubService{}, &stubRetrievalService{}, rag)

	req := httptest.NewRequest(http.MethodPost, "/documents/ask", strings.NewReader(`{bad json`))
	rec := httptest.NewRecorder()

	h.Ask(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestAsk_UnknownField(t *testing.T) {
	rag := &stubRAGService{
		askFn: func(ctx context.Context, question string, limit int) (*service.RAGResponse, error) {
			t.Fatal("service should not be called for unknown fields")
			return nil, nil
		},
	}
	h := New(&stubService{}, &stubRetrievalService{}, rag)

	body := `{"question":"a question","limit":5,"unexpected":"field"}`
	req := httptest.NewRequest(http.MethodPost, "/documents/ask", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Ask(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestAsk_WrongMethod(t *testing.T) {
	rag := &stubRAGService{
		askFn: func(ctx context.Context, question string, limit int) (*service.RAGResponse, error) {
			t.Fatal("service should not be called for wrong method")
			return nil, nil
		},
	}
	h := New(&stubService{}, &stubRetrievalService{}, rag)

	req := httptest.NewRequest(http.MethodGet, "/documents/ask", nil)
	rec := httptest.NewRecorder()

	h.Ask(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", rec.Code)
	}
}

func TestAsk_NoSources(t *testing.T) {
	rag := &stubRAGService{
		askFn: func(ctx context.Context, question string, limit int) (*service.RAGResponse, error) {
			return &service.RAGResponse{
				Answer: "I don't have enough information to answer that.",
				Chunks: []*model.DocumentChunk{},
			}, nil
		},
	}
	h := New(&stubService{}, &stubRetrievalService{}, rag)

	body := `{"question":"an unanswerable question","limit":5}`
	req := httptest.NewRequest(http.MethodPost, "/documents/ask", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Ask(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 even with no sources, got %d", rec.Code)
	}

	var got askResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got.Sources) != 0 {
		t.Errorf("expected empty sources array, got %d", len(got.Sources))
	}
}
