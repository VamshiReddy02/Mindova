package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	"github.com/vamshireddy02/mindova/services/document-service/model"
)

const embeddingDimensions = 8

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

func (r *ChunkRepo) CreateBatch(ctx context.Context, chunks []*model.DocumentChunk) error {
	if len(chunks) == 0 {
		return nil
	}

	for i, c := range chunks {
		if len(c.Embedding) != embeddingDimensions {
			return fmt.Errorf(
				"chunk %d: embedding has %d dimensions, expected %d",
				i, len(c.Embedding), embeddingDimensions,
			)
		}
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) // no-op once Commit succeeds

	const query = `
		INSERT INTO document_chunks (id, document_id, chunk_index, content, embedding)
		VALUES (gen_random_uuid(), $1, $2, $3, $4)
		RETURNING id, created_at
	`

	for _, c := range chunks {
		vec := pgvector.NewVector(c.Embedding)
		if err := tx.QueryRow(ctx, query, c.DocumentID, c.ChunkIndex, c.Content, vec).Scan(&c.ID, &c.CreatedAt); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *ChunkRepo) GetByDocumentID(ctx context.Context, documentID string) ([]*model.DocumentChunk, error) {
	const query = `
		SELECT id, document_id, chunk_index, content, embedding, created_at
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
		var vec pgvector.Vector

		if err := rows.Scan(
			&c.ID,
			&c.DocumentID,
			&c.ChunkIndex,
			&c.Content,
			&vec,
			&c.CreatedAt,
		); err != nil {
			return nil, err
		}

		c.Embedding = vec.Slice()
		chunks = append(chunks, c)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return chunks, nil
}

var _ ChunkRepository = (*ChunkRepo)(nil)
