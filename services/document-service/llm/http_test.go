package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPClient_Success(t *testing.T) {
	var gotBody httpChatCompletionRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("server failed to decode request body: %v", err)
		}

		resp := httpChatCompletionResponse{
			Message: httpChatMessage{Role: "assistant", Content: "the generated answer"},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewHTTPClient(server.URL, nil)

	messages := []Message{
		{Role: "system", Content: "Context: some retrieved chunks."},
		{Role: "user", Content: "What is Mindova?"},
	}

	answer, err := c.Complete(context.Background(), messages)
	if err != nil {
		t.Fatalf("Complete() returned error: %v", err)
	}

	if answer != "the generated answer" {
		t.Errorf("expected %q, got %q", "the generated answer", answer)
	}

	if len(gotBody.Messages) != 2 {
		t.Fatalf("expected server to receive 2 messages, got %d", len(gotBody.Messages))
	}
	if gotBody.Messages[0].Role != "system" || gotBody.Messages[1].Role != "user" {
		t.Errorf("expected roles [system, user], got [%s, %s]", gotBody.Messages[0].Role, gotBody.Messages[1].Role)
	}
}

func TestHTTPClient_EmptyMessages_NoRequestMade(t *testing.T) {
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewHTTPClient(server.URL, nil)

	_, err := c.Complete(context.Background(), nil)
	if !errors.Is(err, ErrEmptyMessages) {
		t.Fatalf("expected ErrEmptyMessages, got %v", err)
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request for empty messages, got %d", requestCount)
	}
}

func TestHTTPClient_ConnectionFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	url := server.URL
	server.Close() // guarantees connection refused

	c := NewHTTPClient(url, nil)

	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "hello"}})
	if err == nil {
		t.Fatal("expected a connection error, got nil")
	}
}

func TestHTTPClient_NonSuccessStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"detail":"llm provider unavailable"}`))
	}))
	defer server.Close()

	c := NewHTTPClient(server.URL, nil)

	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "hello"}})
	if err == nil {
		t.Fatal("expected an error for a 500 response, got nil")
	}
}

func TestHTTPClient_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`not valid json{{{`))
	}))
	defer server.Close()

	c := NewHTTPClient(server.URL, nil)

	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "hello"}})
	if err == nil {
		t.Fatal("expected an error for invalid JSON, got nil")
	}
}

func TestHTTPClient_EmptyAnswer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := httpChatCompletionResponse{Message: httpChatMessage{Role: "assistant", Content: ""}}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewHTTPClient(server.URL, nil)

	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "hello"}})
	if err == nil {
		t.Fatal("expected an error for an empty answer, got nil")
	}
}
