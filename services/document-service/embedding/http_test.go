package embedding

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPEmbedder_Success(t *testing.T) {
	var gotBody httpEmbeddingRequest
	var gotContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("server failed to decode request body: %v", err)
		}

		resp := httpEmbeddingResponse{
			Embeddings: [][]float32{
				{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8},
				{0.9, 0.8, 0.7, 0.6, 0.5, 0.4, 0.3, 0.2},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	e := NewHTTPEmbedder(server.URL, nil)

	texts := []string{"first text", "second text"}
	embeddings, err := e.Embed(context.Background(), texts)
	if err != nil {
		t.Fatalf("Embed() returned error: %v", err)
	}

	if len(embeddings) != 2 {
		t.Fatalf("expected 2 embeddings, got %d", len(embeddings))
	}
	if len(embeddings[0]) != 8 || len(embeddings[1]) != 8 {
		t.Errorf("expected 8-dimension embeddings, got %d and %d", len(embeddings[0]), len(embeddings[1]))
	}

	if len(gotBody.Texts) != 2 || gotBody.Texts[0] != "first text" || gotBody.Texts[1] != "second text" {
		t.Errorf("expected server to receive texts %v, got %v", texts, gotBody.Texts)
	}
	if gotContentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", gotContentType)
	}
}

func TestHTTPEmbedder_EmptyInput_NoRequestMade(t *testing.T) {
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	e := NewHTTPEmbedder(server.URL, nil)

	_, err := e.Embed(context.Background(), []string{})
	if !errors.Is(err, ErrEmptyTexts) {
		t.Fatalf("expected ErrEmptyTexts, got %v", err)
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request for empty input, got %d", requestCount)
	}
}

func TestHTTPEmbedder_ConnectionFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	url := server.URL
	server.Close() // shut down before making the request, guaranteeing connection refused

	e := NewHTTPEmbedder(url, nil)

	_, err := e.Embed(context.Background(), []string{"hello"})
	if err == nil {
		t.Fatal("expected a connection error, got nil")
	}
}

func TestHTTPEmbedder_NonSuccessStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"detail":"provider unavailable"}`))
	}))
	defer server.Close()

	e := NewHTTPEmbedder(server.URL, nil)

	_, err := e.Embed(context.Background(), []string{"hello"})
	if err == nil {
		t.Fatal("expected an error for a 500 response, got nil")
	}
}

func TestHTTPEmbedder_NonSuccessStatus_400(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"detail":"texts must not be empty"}`))
	}))
	defer server.Close()

	e := NewHTTPEmbedder(server.URL, nil)

	_, err := e.Embed(context.Background(), []string{"hello"})
	if err == nil {
		t.Fatal("expected an error for a 400 response, got nil")
	}
}

func TestHTTPEmbedder_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`not valid json{{{`))
	}))
	defer server.Close()

	e := NewHTTPEmbedder(server.URL, nil)

	_, err := e.Embed(context.Background(), []string{"hello"})
	if err == nil {
		t.Fatal("expected an error for invalid JSON, got nil")
	}
}

func TestHTTPEmbedder_EmbeddingCountMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := httpEmbeddingResponse{
			Embeddings: [][]float32{
				{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8}, // only 1, but 2 texts were sent
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	e := NewHTTPEmbedder(server.URL, nil)

	_, err := e.Embed(context.Background(), []string{"first text", "second text"})
	if err == nil {
		t.Fatal("expected an error for embedding count mismatch, got nil")
	}
}

func TestHTTPEmbedder_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // block until the client cancels
	}))
	defer server.Close()

	e := NewHTTPEmbedder(server.URL, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the call, so it fails fast rather than hanging

	_, err := e.Embed(ctx, []string{"hello"})
	if err == nil {
		t.Fatal("expected an error for a cancelled context, got nil")
	}
}

func TestHTTPEmbedder_TrimsTrailingSlashFromBaseURL(t *testing.T) {
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		resp := httpEmbeddingResponse{Embeddings: [][]float32{{0, 0, 0, 0, 0, 0, 0, 0}}}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Pass a trailing slash and confirm it doesn't produce a double slash
	// in the request path.
	e := NewHTTPEmbedder(server.URL+"/", nil)

	if _, err := e.Embed(context.Background(), []string{"hello"}); err != nil {
		t.Fatalf("Embed() returned error: %v", err)
	}

	if gotPath != "/v1/embeddings" {
		t.Errorf("expected path /v1/embeddings, got %q", gotPath)
	}
}
