package worker

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vamshireddy02/mindova/services/document-service/model"
)

func TestProcess_ShortContent_ReturnsSingleChunk(t *testing.T) {
	p := NewTextProcessor(1000)
	doc := &model.Document{ID: "doc-1", Content: "Mindova is an AI knowledge platform."}

	chunks, err := p.Process(context.Background(), doc)
	if err != nil {
		t.Fatalf("Process() returned error: %v", err)
	}

	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d: %v", len(chunks), chunks)
	}
	if chunks[0] != "Mindova is an AI knowledge platform." {
		t.Errorf("expected chunk to equal normalized content, got %q", chunks[0])
	}
}

func TestProcess_LongContent_SplitsIntoMultipleChunks(t *testing.T) {
	// 50 words of 5 chars each plus spaces; with a small chunk size this
	// must produce more than one chunk, and no chunk may exceed the size.
	words := make([]string, 50)
	for i := range words {
		words[i] = "abcde"
	}
	content := strings.Join(words, " ")

	const chunkSize = 30
	p := NewTextProcessor(chunkSize)
	doc := &model.Document{Content: content}

	chunks, err := p.Process(context.Background(), doc)
	if err != nil {
		t.Fatalf("Process() returned error: %v", err)
	}

	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}

	for i, c := range chunks {
		if len(c) > chunkSize {
			t.Errorf("chunk %d exceeds chunkSize %d: len=%d, content=%q", i, chunkSize, len(c), c)
		}
	}
}

func TestProcess_PreservesWordOrderAndContent(t *testing.T) {
	content := "one two three four five six seven eight nine ten"
	p := NewTextProcessor(15) // small enough to force multiple chunks

	doc := &model.Document{Content: content}
	chunks, err := p.Process(context.Background(), doc)
	if err != nil {
		t.Fatalf("Process() returned error: %v", err)
	}

	var reconstructed []string
	for _, c := range chunks {
		reconstructed = append(reconstructed, strings.Fields(c)...)
	}

	original := strings.Fields(content)
	if len(reconstructed) != len(original) {
		t.Fatalf("expected %d words reconstructed, got %d", len(original), len(reconstructed))
	}
	for i := range original {
		if reconstructed[i] != original[i] {
			t.Errorf("word %d: expected %q, got %q", i, original[i], reconstructed[i])
		}
	}
}

func TestProcess_SingleWordLongerThanChunkSize_KeptWhole(t *testing.T) {
	longWord := strings.Repeat("a", 50)
	p := NewTextProcessor(10) // smaller than the word itself

	doc := &model.Document{Content: longWord}
	chunks, err := p.Process(context.Background(), doc)
	if err != nil {
		t.Fatalf("Process() returned error: %v", err)
	}

	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for a single oversized word, got %d", len(chunks))
	}
	if chunks[0] != longWord {
		t.Errorf("expected the oversized word kept whole, got %q", chunks[0])
	}
}

func TestProcess_NormalizesCRLFLineEndings(t *testing.T) {
	p := NewTextProcessor(1000)
	doc := &model.Document{Content: "line one\r\nline two\r\nline three"}

	chunks, err := p.Process(context.Background(), doc)
	if err != nil {
		t.Fatalf("Process() returned error: %v", err)
	}

	joined := strings.Join(chunks, " ")
	if strings.Contains(joined, "\r") {
		t.Errorf("expected no carriage returns in output, got %q", joined)
	}
}

func TestProcess_TrimsTrailingWhitespacePerLine(t *testing.T) {
	// Trailing whitespace shouldn't survive normalization, though since
	// chunking uses strings.Fields (which already splits on any
	// whitespace), the real assertion here is that normalize() doesn't
	// error or panic on trailing-whitespace input and content still comes
	// through intact.
	p := NewTextProcessor(1000)
	doc := &model.Document{Content: "hello   \nworld   "}

	chunks, err := p.Process(context.Background(), doc)
	if err != nil {
		t.Fatalf("Process() returned error: %v", err)
	}
	if len(chunks) != 1 || chunks[0] != "hello world" {
		t.Errorf("expected single chunk %q, got %v", "hello world", chunks)
	}
}

func TestProcess_CollapsesExcessBlankLines(t *testing.T) {
	got := normalize("paragraph one\n\n\n\n\nparagraph two")
	want := "paragraph one\n\nparagraph two"

	if got != want {
		t.Errorf("expected blank lines collapsed to %q, got %q", want, got)
	}
}

func TestProcess_EmptyContent_ReturnsError(t *testing.T) {
	p := NewTextProcessor(1000)
	doc := &model.Document{Content: ""}

	_, err := p.Process(context.Background(), doc)
	if !errors.Is(err, ErrEmptyContent) {
		t.Fatalf("expected ErrEmptyContent, got %v", err)
	}
}

func TestProcess_WhitespaceOnlyContent_ReturnsError(t *testing.T) {
	p := NewTextProcessor(1000)
	doc := &model.Document{Content: "   \n\n\t  \n  "}

	_, err := p.Process(context.Background(), doc)
	if !errors.Is(err, ErrEmptyContent) {
		t.Fatalf("expected ErrEmptyContent, got %v", err)
	}
}

func TestProcess_NilDocument_ReturnsError(t *testing.T) {
	p := NewTextProcessor(1000)

	_, err := p.Process(context.Background(), nil)
	if !errors.Is(err, ErrNilDocument) {
		t.Fatalf("expected ErrNilDocument, got %v", err)
	}
}

func TestNewTextProcessor_DefaultsChunkSizeWhenInvalid(t *testing.T) {
	p := NewTextProcessor(0)
	if p.chunkSize != defaultChunkSize {
		t.Errorf("expected default chunk size %d, got %d", defaultChunkSize, p.chunkSize)
	}

	p = NewTextProcessor(-100)
	if p.chunkSize != defaultChunkSize {
		t.Errorf("expected default chunk size %d for negative input, got %d", defaultChunkSize, p.chunkSize)
	}

	p = NewTextProcessor(500)
	if p.chunkSize != 500 {
		t.Errorf("expected chunk size 500 to be honored, got %d", p.chunkSize)
	}
}

func TestProcess_MarkdownContent_ChunksSuccessfully(t *testing.T) {
	p := NewTextProcessor(1000)
	doc := &model.Document{
		Content: "# Architecture\n\nMindova is built as a **Go monorepo**.\n\n" +
			"## Services\n\n- document-service\n- worker",
	}

	chunks, err := p.Process(context.Background(), doc)
	if err != nil {
		t.Fatalf("Process() returned error: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk for markdown content")
	}
}
