package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vamshireddy02/mindova/services/document-service/model"
)

// ChunkRepository defines the persistence operations for document chunks.
type ChunkRepository interface {
	CreateBatch(ctx context.Context, chunks []*model.DocumentChunk) error
	GetByDocumentID(ctx context.Context, documentID string) ([]*model.DocumentChunk, error)
}

// ChunkRepo is the PostgreSQL-backed implementation of ChunkRepository.
type ChunkRepo struct {
	db *pgxpool.Pool
}

// NewChunkRepo creates a ChunkRepo backed by the given connection pool.
func NewChunkRepo(db *pgxpool.Pool) *ChunkRepo {
	return &ChunkRepo{db: db}
}

// CreateBatch inserts all chunks in a single transaction: either every
// chunk is persisted, or none are. This matters for the worker's use
// case — a document's chunks should never end up partially stored if
// something fails midway through the batch.
//
// PostgreSQL generates each chunk's ID and created_at; CreateBatch
// populates them back onto the corresponding chunk.
//
// Calling CreateBatch with an empty slice is a no-op.
func (r *ChunkRepo) CreateBatch(ctx context.Context, chunks []*model.DocumentChunk) error {
	if len(chunks) == 0 {
		return nil
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) // no-op once Commit succeeds

	const query = `
		INSERT INTO document_chunks (id, document_id, chunk_index, content)
		VALUES (gen_random_uuid(), $1, $2, $3)
		RETURNING id, created_at
	`

	for _, c := range chunks {
		if err := tx.QueryRow(ctx, query, c.DocumentID, c.ChunkIndex, c.Content).Scan(&c.ID, &c.CreatedAt); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// GetByDocumentID fetches all chunks for a document, ordered by their
// original position (chunk_index ascending).
func (r *ChunkRepo) GetByDocumentID(ctx context.Context, documentID string) ([]*model.DocumentChunk, error) {
	const query = `
		SELECT id, document_id, chunk_index, content, created_at
		FROM document_chunks
		WHERE document_id = $1
		ORDER BY chunk_index ASC
	`

	rows, err := r.db.Query(ctx, query, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	chunks := make([]*model.DocumentChunk, 0)

	for rows.Next() {
		c := &model.DocumentChunk{}
		if err := rows.Scan(
			&c.ID,
			&c.DocumentID,
			&c.ChunkIndex,
			&c.Content,
			&c.CreatedAt,
		); err != nil {
			return nil, err
		}
		chunks = append(chunks, c)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return chunks, nil
}

// Compile-time check that *ChunkRepo satisfies ChunkRepository.
var _ ChunkRepository = (*ChunkRepo)(nil)
