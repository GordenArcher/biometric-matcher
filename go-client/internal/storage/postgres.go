package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"

	"github.com/GordenArcher/biometric-matcher/internal/crypto"
)

// Store is the only thing in this codebase that should ever see ciphertext
// on its way in or out of Postgres. Callers give it plaintext templates
// and get plaintext templates back, encryption is an implementation
// detail of this type, not something command code should have to think
// about on every call.
type Store struct {
	db  *sql.DB
	enc *crypto.Encryptor
}

func Open(ctx context.Context, dsn string, enc *crypto.Encryptor) (*Store, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres connection: %w", err)
	}

	// Fail fast on a bad DSN rather than letting the first real query be
	// what surfaces a typo'd connection string.
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return &Store{db: db, enc: enc}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// TemplateRecord pairs a person's ID with their decrypted template,
// this is the shape Identify needs to build a candidate batch, it is
// never returned with ciphertext still attached.
type TemplateRecord struct {
	PersonID string
	Template []byte
}

// EnrollPerson writes both the biographic row and the encrypted template
// in one transaction, a person should never exist without a template or
// vice versa, a partial write here would leave an unusable identity
// sitting in the register.
func (s *Store) EnrollPerson(ctx context.Context, fullName string, dateOfBirth time.Time, plaintextTemplate []byte) (string, error) {
	ciphertext, err := s.enc.Encrypt(ctx, plaintextTemplate)
	if err != nil {
		return "", fmt.Errorf("encrypt template: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin transaction: %w", err)
	}
	// Rollback is a no-op after a successful Commit, safe to defer
	// unconditionally rather than tracking whether commit happened.
	defer tx.Rollback()

	var personID string
	err = tx.QueryRowContext(ctx,
		`INSERT INTO people (full_name, date_of_birth) VALUES ($1, $2) RETURNING id`,
		fullName, dateOfBirth,
	).Scan(&personID)
	if err != nil {
		return "", fmt.Errorf("insert person: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO biometric_templates (person_id, ciphertext) VALUES ($1, $2)`,
		personID, ciphertext,
	)
	if err != nil {
		return "", fmt.Errorf("insert template: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit transaction: %w", err)
	}

	return personID, nil
}

// GetTemplate fetches and decrypts the most recently enrolled template
// for a person. Most recent rather than "the" template since a person
// could in principle be re-enrolled (e.g. after the kind of card
// correction this whole project started from), older templates are kept
// rather than overwritten so there is an audit trail, not just a mutable
// single row.
func (s *Store) GetTemplate(ctx context.Context, personID string) ([]byte, error) {
	var ciphertext []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT ciphertext FROM biometric_templates
		 WHERE person_id = $1
		 ORDER BY created_at DESC
		 LIMIT 1`,
		personID,
	).Scan(&ciphertext)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no template found for person %s", personID)
		}
		return nil, fmt.Errorf("query template: %w", err)
	}

	plaintext, err := s.enc.Decrypt(ctx, ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypt template: %w", err)
	}

	return plaintext, nil
}

// ListTemplates pages through the register for Identify's candidate
// batch. Go owns pagination, not the matcher, per the reasoning in
// proto/biometric.proto, this is where that actually happens.
func (s *Store) ListTemplates(ctx context.Context, limit, offset int) ([]TemplateRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT ON (person_id) person_id, ciphertext
		 FROM biometric_templates
		 ORDER BY person_id, created_at DESC
		 LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("query templates: %w", err)
	}
	defer rows.Close()

	var records []TemplateRecord
	for rows.Next() {
		var personID string
		var ciphertext []byte
		if err := rows.Scan(&personID, &ciphertext); err != nil {
			return nil, fmt.Errorf("scan template row: %w", err)
		}

		plaintext, err := s.enc.Decrypt(ctx, ciphertext)
		if err != nil {
			return nil, fmt.Errorf("decrypt template for person %s: %w", personID, err)
		}

		records = append(records, TemplateRecord{PersonID: personID, Template: plaintext})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate template rows: %w", err)
	}

	return records, nil
}
