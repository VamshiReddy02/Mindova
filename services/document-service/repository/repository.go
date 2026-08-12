package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vamshireddy02/mindova/services/document-service/model"
)

var ErrNotFound = errors.New("document not found")

type DocumentRepository interface {
	Create(ctx context.Context, doc *model.Document) error
	GetByID(ctx context.Context, id string) (*model.Document, error)
	List(ctx context.Context, limit int) ([]*model.Document, error)
	Update(ctx context.Context, doc *model.Document) error
	Delete(ctx context.Context, id string) error
}

type Repository struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, doc *model.Document) error {
	const query = `
		INSERT INTO documents (name, content, content_type)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at
	`

	return r.db.QueryRow(ctx, query,
		doc.Name,
		doc.Content,
		doc.ContentType,
	).Scan(
		&doc.ID,
		&doc.CreatedAt,
		&doc.UpdatedAt,
	)
}

func (r *Repository) GetByID(ctx context.Context, id string) (*model.Document, error) {
	const query = `
		SELECT id, name, content, content_type, created_at, updated_at
		FROM documents
		WHERE id = $1
	`

	doc := &model.Document{}

	err := r.db.QueryRow(ctx, query, id).Scan(
		&doc.ID,
		&doc.Name,
		&doc.Content,
		&doc.ContentType,
		&doc.CreatedAt,
		&doc.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return doc, nil
}

func (r *Repository) List(ctx context.Context, limit int) ([]*model.Document, error) {
	const query = `
		SELECT id, name, content, content_type, created_at, updated_at
		FROM documents
		ORDER BY created_at DESC
		LIMIT $1
	`

	rows, err := r.db.Query(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	docs := make([]*model.Document, 0)

	for rows.Next() {
		doc := &model.Document{}
		if err := rows.Scan(
			&doc.ID,
			&doc.Name,
			&doc.Content,
			&doc.ContentType,
			&doc.CreatedAt,
			&doc.UpdatedAt,
		); err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return docs, nil
}

func (r *Repository) Update(ctx context.Context, doc *model.Document) error {
	const query = `
		UPDATE documents
		SET name = $1, content = $2, content_type = $3, updated_at = now()
		WHERE id = $4
		RETURNING created_at, updated_at
	`

	err := r.db.QueryRow(ctx, query,
		doc.Name,
		doc.Content,
		doc.ContentType,
		doc.ID,
	).Scan(&doc.CreatedAt, &doc.UpdatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	const query = `DELETE FROM documents WHERE id = $1`

	tag, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
