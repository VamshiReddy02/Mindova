package worker

import (
	"context"
	"errors"
	"strings"

	"github.com/vamshireddy02/mindova/services/document-service/model"
)

// DocumentProcessor turns a document's raw content into a slice of text
// chunks, ready for a later step (embeddings) to consume. This first
// version handles plain text and Markdown only — normalization and
// chunking, nothing content-type-aware yet.
type DocumentProcessor interface {
	Process(ctx context.Context, doc *model.Document) ([]string, error)
}

// defaultChunkSize is the maximum number of characters per chunk, used
// when NewTextProcessor is given a non-positive size.
const defaultChunkSize = 1000

// ErrNilDocument is returned when Process is called with a nil document.
var ErrNilDocument = errors.New("processor: document is nil")

// ErrEmptyContent is returned when a document has no content to process
// (empty, or only whitespace) after normalization.
var ErrEmptyContent = errors.New("processor: document has no content to process")

// TextProcessor is a DocumentProcessor for plain text and Markdown
// content. It normalizes whitespace and line endings, then splits the
// result into chunks of up to chunkSize characters without splitting any
// individual word.
type TextProcessor struct {
	chunkSize int
}

// NewTextProcessor creates a TextProcessor. chunkSize is the maximum
// number of characters per chunk; pass 0 to use the default (1000).
func NewTextProcessor(chunkSize int) *TextProcessor {
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}
	return &TextProcessor{chunkSize: chunkSize}
}

// Process normalizes doc.Content and splits it into chunks.
func (p *TextProcessor) Process(ctx context.Context, doc *model.Document) ([]string, error) {
	if doc == nil {
		return nil, ErrNilDocument
	}

	normalized := normalize(doc.Content)
	if normalized == "" {
		return nil, ErrEmptyContent
	}

	return chunk(normalized, p.chunkSize), nil
}

// normalize standardizes line endings, strips trailing whitespace from
// each line, collapses three or more consecutive blank lines down to a
// single paragraph break, and trims leading/trailing whitespace overall.
func normalize(content string) string {
	// Standardize line endings to \n.
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	// Strip trailing whitespace from each line.
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	content = strings.Join(lines, "\n")

	// Collapse 3+ consecutive newlines (2+ blank lines) into exactly two
	// newlines (a single blank line / paragraph break).
	for strings.Contains(content, "\n\n\n") {
		content = strings.ReplaceAll(content, "\n\n\n", "\n\n")
	}

	return strings.TrimSpace(content)
}

// chunk splits content into chunks of at most size characters, breaking
// only on whitespace so no word is ever split across two chunks. A single
// word longer than size is kept whole in its own (oversized) chunk rather
// than being cut.
func chunk(content string, size int) []string {
	words := strings.Fields(content)
	if len(words) == 0 {
		return nil
	}

	chunks := make([]string, 0)
	var current strings.Builder

	for _, word := range words {
		switch {
		case current.Len() == 0:
			current.WriteString(word)

		case current.Len()+1+len(word) <= size:
			current.WriteByte(' ')
			current.WriteString(word)

		default:
			chunks = append(chunks, current.String())
			current.Reset()
			current.WriteString(word)
		}
	}

	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}

	return chunks
}

// Compile-time check that *TextProcessor satisfies DocumentProcessor.
var _ DocumentProcessor = (*TextProcessor)(nil)
