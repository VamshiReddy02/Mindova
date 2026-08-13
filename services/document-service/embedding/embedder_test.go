package embedding

import (
	"context"
	"errors"
	"testing"
)

func TestMockEmbedder_ReturnsOneVectorPerText(t *testing.T) {
	e := &MockEmbedder{}
	texts := []string{"first chunk", "second chunk", "third chunk"}

	vectors, err := e.Embed(context.Background(), texts)
	if err != nil {
		t.Fatalf("Embed() returned error: %v", err)
	}

	if len(vectors) != len(texts) {
		t.Fatalf("expected %d vectors, got %d", len(texts), len(vectors))
	}
}

func TestMockEmbedder_DefaultDimensions(t *testing.T) {
	e := &MockEmbedder{}

	vectors, err := e.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("Embed() returned error: %v", err)
	}

	if len(vectors[0]) != defaultDimensions {
		t.Errorf("expected default dimensions %d, got %d", defaultDimensions, len(vectors[0]))
	}
}

func TestMockEmbedder_CustomDimensions(t *testing.T) {
	e := &MockEmbedder{Dimensions: 16}

	vectors, err := e.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("Embed() returned error: %v", err)
	}

	if len(vectors[0]) != 16 {
		t.Errorf("expected 16 dimensions, got %d", len(vectors[0]))
	}
}

func TestMockEmbedder_DeterministicForSameText(t *testing.T) {
	e := &MockEmbedder{}

	v1, err := e.Embed(context.Background(), []string{"same text"})
	if err != nil {
		t.Fatalf("Embed() returned error: %v", err)
	}
	v2, err := e.Embed(context.Background(), []string{"same text"})
	if err != nil {
		t.Fatalf("Embed() returned error: %v", err)
	}

	if len(v1[0]) != len(v2[0]) {
		t.Fatalf("vector length mismatch: %d vs %d", len(v1[0]), len(v2[0]))
	}
	for i := range v1[0] {
		if v1[0][i] != v2[0][i] {
			t.Fatalf("expected identical vectors for identical text, differed at dimension %d: %v vs %v", i, v1[0], v2[0])
		}
	}
}

func TestMockEmbedder_DifferentTextsProduceDifferentVectors(t *testing.T) {
	e := &MockEmbedder{}

	vectors, err := e.Embed(context.Background(), []string{"alpha", "beta"})
	if err != nil {
		t.Fatalf("Embed() returned error: %v", err)
	}

	identical := true
	for i := range vectors[0] {
		if vectors[0][i] != vectors[1][i] {
			identical = false
			break
		}
	}
	if identical {
		t.Error("expected different texts to produce different vectors, got identical vectors")
	}
}

func TestMockEmbedder_PreservesInputOrder(t *testing.T) {
	e := &MockEmbedder{}
	texts := []string{"one", "two", "three"}

	vectors, err := e.Embed(context.Background(), texts)
	if err != nil {
		t.Fatalf("Embed() returned error: %v", err)
	}

	// Each position's vector should match embedding that same text alone.
	for i, text := range texts {
		solo, err := e.Embed(context.Background(), []string{text})
		if err != nil {
			t.Fatalf("Embed() returned error: %v", err)
		}
		for d := range solo[0] {
			if vectors[i][d] != solo[0][d] {
				t.Errorf("position %d (%q): expected vector to match solo embedding, differed at dimension %d", i, text, d)
				break
			}
		}
	}
}

func TestMockEmbedder_EmptyTexts_ReturnsError(t *testing.T) {
	e := &MockEmbedder{}

	_, err := e.Embed(context.Background(), nil)
	if !errors.Is(err, ErrEmptyTexts) {
		t.Fatalf("expected ErrEmptyTexts, got %v", err)
	}

	_, err = e.Embed(context.Background(), []string{})
	if !errors.Is(err, ErrEmptyTexts) {
		t.Fatalf("expected ErrEmptyTexts for empty slice, got %v", err)
	}
}

func TestMockEmbedder_ConfiguredError_IsReturned(t *testing.T) {
	wantErr := errors.New("simulated provider outage")
	e := &MockEmbedder{Err: wantErr}

	_, err := e.Embed(context.Background(), []string{"hello"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error %v, got %v", wantErr, err)
	}
}
