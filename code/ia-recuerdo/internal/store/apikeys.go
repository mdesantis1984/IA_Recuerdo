package store

import (
	"context"
	"database/sql"
	"time"
)

// APIKey is a minimal view of an api_keys row (key_hash is never returned to callers).
type APIKey struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Scopes    string     `json:"scopes"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Revoked   bool       `json:"revoked"`
}

// CreateAPIKey persists a new API key (key_hash = SHA-256 of the raw key).
func (s *Store) CreateAPIKey(ctx context.Context, id, name, keyHash, scopes string) error {
	now := time.Now().UTC()
	if s.pg() {
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO api_keys(id,name,key_hash,scopes,created_at) VALUES($1,$2,$3,$4,$5)`,
			id, name, keyHash, scopes, now)
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO api_keys(id,name,key_hash,scopes,created_at) VALUES(?,?,?,?,?)`,
		id, name, keyHash, scopes, now.Unix())
	return err
}

// ValidateAPIKey returns true if keyHash matches a non-revoked, non-expired key.
func (s *Store) ValidateAPIKey(ctx context.Context, keyHash string) (bool, error) {
	var revoked int
	var expiresAt sql.NullInt64

	if s.pg() {
		var expiresPg sql.NullTime
		err := s.db.QueryRowContext(ctx,
			`SELECT revoked::int, expires_at FROM api_keys WHERE key_hash=$1`, keyHash,
		).Scan(&revoked, &expiresPg)
		if err == sql.ErrNoRows {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if revoked == 1 {
			return false, nil
		}
		if expiresPg.Valid && expiresPg.Time.Before(time.Now().UTC()) {
			return false, nil
		}
		return true, nil
	}

	err := s.db.QueryRowContext(ctx,
		`SELECT revoked, expires_at FROM api_keys WHERE key_hash=?`, keyHash,
	).Scan(&revoked, &expiresAt)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if revoked == 1 {
		return false, nil
	}
	if expiresAt.Valid && time.Unix(expiresAt.Int64, 0).UTC().Before(time.Now().UTC()) {
		return false, nil
	}
	return true, nil
}

// ListAPIKeys returns all non-revoked API keys (without key_hash).
func (s *Store) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	var rows *sql.Rows
	var err error
	if s.pg() {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id,name,scopes,created_at,expires_at,revoked FROM api_keys ORDER BY created_at DESC`)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id,name,scopes,created_at,expires_at,revoked FROM api_keys ORDER BY created_at DESC`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		k := APIKey{}
		if s.pg() {
			var createdAt time.Time
			var expiresAt sql.NullTime
			var revoked bool
			if err := rows.Scan(&k.ID, &k.Name, &k.Scopes, &createdAt, &expiresAt, &revoked); err != nil {
				return nil, err
			}
			k.CreatedAt = createdAt
			if expiresAt.Valid {
				t := expiresAt.Time
				k.ExpiresAt = &t
			}
			k.Revoked = revoked
		} else {
			var createdAt int64
			var expiresAt sql.NullInt64
			var revoked int
			if err := rows.Scan(&k.ID, &k.Name, &k.Scopes, &createdAt, &expiresAt, &revoked); err != nil {
				return nil, err
			}
			k.CreatedAt = time.Unix(createdAt, 0).UTC()
			if expiresAt.Valid {
				t := time.Unix(expiresAt.Int64, 0).UTC()
				k.ExpiresAt = &t
			}
			k.Revoked = revoked == 1
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// RevokeAPIKey marks a key as revoked by its ID.
func (s *Store) RevokeAPIKey(ctx context.Context, id string) error {
	if s.pg() {
		_, err := s.db.ExecContext(ctx, `UPDATE api_keys SET revoked=TRUE WHERE id=$1`, id)
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE api_keys SET revoked=1 WHERE id=?`, id)
	return err
}
