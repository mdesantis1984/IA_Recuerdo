package store

import (
	"context"
	"database/sql"
	"fmt"
)

// migrate runs all pending schema migrations.
func (s *Store) migrate(ctx context.Context) error {
	if err := s.createMigrationsTable(ctx); err != nil {
		return err
	}

	migrations := []struct {
		version string
		up      func(context.Context, *sql.Tx) error
	}{
		{"v1_init", s.v1Init},
		{"v2_pgvector", s.v2Pgvector},
		{"v3_content_split", s.v3ContentSplit},
		{"v4_relations_and_attachments", s.v4RelationsAndAttachments},
		{"v5_repair_text_encoding", s.v5RepairTextEncoding},
	}

	for _, m := range migrations {
		var applied bool
		if s.pg() {
			_ = s.db.QueryRowContext(ctx, "SELECT true FROM schema_migrations WHERE version=$1", m.version).Scan(&applied)
		} else {
			_ = s.db.QueryRowContext(ctx, "SELECT 1 FROM schema_migrations WHERE version=?", m.version).Scan(&applied)
		}
		if applied {
			continue
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin tx %s: %w", m.version, err)
		}
		if err := m.up(ctx, tx); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s: %w", m.version, err)
		}
		var q string
		if s.pg() {
			q = "INSERT INTO schema_migrations(version,applied_at) VALUES($1,NOW())"
		} else {
			q = "INSERT INTO schema_migrations(version,applied_at) VALUES(?,strftime('%s','now'))"
		}
		if _, err := tx.ExecContext(ctx, q, m.version); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %s: %w", m.version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", m.version, err)
		}
	}
	return nil
}

func (s *Store) createMigrationsTable(ctx context.Context) error {
	var q string
	if s.pg() {
		q = `CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT        PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`
	} else {
		q = `CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT   PRIMARY KEY,
			applied_at BIGINT NOT NULL DEFAULT (strftime('%s','now'))
		)`
	}
	_, err := s.db.ExecContext(ctx, q)
	return err
}

// v1Init creates all core tables.
func (s *Store) v1Init(ctx context.Context, tx *sql.Tx) error {
	var stmts []string

	if s.pg() {
		stmts = []string{
			`CREATE TABLE IF NOT EXISTS observations (
				id             BIGSERIAL    PRIMARY KEY,
				title          TEXT         NOT NULL,
				content        TEXT         NOT NULL DEFAULT '',
				type           TEXT         NOT NULL DEFAULT 'discovery',
				project        TEXT         NOT NULL DEFAULT 'default',
				scope          TEXT         NOT NULL DEFAULT 'project',
				topic_key      TEXT,
				tags           TEXT         NOT NULL DEFAULT '',
				session_id     TEXT,
				duplicate_count INT         NOT NULL DEFAULT 0,
				revision_count  INT         NOT NULL DEFAULT 0,
				created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
				updated_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
				last_seen_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
				deleted_at     TIMESTAMPTZ
			)`,
			`CREATE INDEX IF NOT EXISTS idx_obs_project       ON observations(project)`,
			`CREATE INDEX IF NOT EXISTS idx_obs_topic_key     ON observations(project, scope, topic_key) WHERE topic_key IS NOT NULL`,
			`CREATE INDEX IF NOT EXISTS idx_obs_last_seen     ON observations(last_seen_at)`,
			`CREATE INDEX IF NOT EXISTS idx_obs_deleted       ON observations(deleted_at) WHERE deleted_at IS NULL`,
			// Full-text search index via tsvector
			`CREATE INDEX IF NOT EXISTS idx_obs_fts ON observations
				USING GIN(to_tsvector('english', title || ' ' || content))`,
			`CREATE TABLE IF NOT EXISTS observation_content (
				observation_id BIGINT      PRIMARY KEY REFERENCES observations(id) ON DELETE CASCADE,
				content        TEXT        NOT NULL,
				created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
			)`,
			`CREATE TABLE IF NOT EXISTS sessions (
				id         TEXT         PRIMARY KEY,
				project    TEXT         NOT NULL DEFAULT 'default',
				agent      TEXT         NOT NULL DEFAULT '',
				goal       TEXT         NOT NULL DEFAULT '',
				summary    TEXT         NOT NULL DEFAULT '',
				started_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
				ended_at   TIMESTAMPTZ
			)`,
			`CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions(project)`,
			`CREATE TABLE IF NOT EXISTS prompts (
				id         BIGSERIAL   PRIMARY KEY,
				project    TEXT        NOT NULL DEFAULT 'default',
				content    TEXT        NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			)`,
			`CREATE INDEX IF NOT EXISTS idx_prompts_project ON prompts(project)`,
			`CREATE TABLE IF NOT EXISTS attachments (
				id             BIGSERIAL   PRIMARY KEY,
				observation_id BIGINT      NOT NULL REFERENCES observations(id) ON DELETE CASCADE,
				name           TEXT        NOT NULL,
				mime           TEXT        NOT NULL DEFAULT 'application/octet-stream',
				size_bytes     BIGINT      NOT NULL DEFAULT 0,
				sha256         TEXT        NOT NULL DEFAULT '',
				data           BYTEA       NOT NULL,
				created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
			)`,
			`CREATE INDEX IF NOT EXISTS idx_attachments_observation ON attachments(observation_id)`,
			`CREATE TABLE IF NOT EXISTS observation_relations (
				id         BIGSERIAL   PRIMARY KEY,
				from_id    BIGINT      NOT NULL REFERENCES observations(id) ON DELETE CASCADE,
				to_id      BIGINT      NOT NULL REFERENCES observations(id) ON DELETE CASCADE,
				type       TEXT        NOT NULL DEFAULT 'related',
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				UNIQUE(from_id, to_id, type)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_observation_relations_from ON observation_relations(from_id)`,
			`CREATE INDEX IF NOT EXISTS idx_observation_relations_to ON observation_relations(to_id)`,
			`CREATE TABLE IF NOT EXISTS api_keys (
				id         TEXT    PRIMARY KEY,
				name       TEXT    NOT NULL,
				key_hash   TEXT    NOT NULL UNIQUE,
				scopes     TEXT    NOT NULL DEFAULT 'read,write',
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				expires_at TIMESTAMPTZ,
				revoked    BOOLEAN NOT NULL DEFAULT FALSE
			)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash)`,
		}
	} else {
		stmts = []string{
			`CREATE TABLE IF NOT EXISTS observations (
				id              INTEGER PRIMARY KEY AUTOINCREMENT,
				title           TEXT    NOT NULL,
				content         TEXT    NOT NULL DEFAULT '',
				type            TEXT    NOT NULL DEFAULT 'discovery',
				project         TEXT    NOT NULL DEFAULT 'default',
				scope           TEXT    NOT NULL DEFAULT 'project',
				topic_key       TEXT,
				tags            TEXT    NOT NULL DEFAULT '',
				session_id      TEXT,
				duplicate_count INTEGER NOT NULL DEFAULT 0,
				revision_count  INTEGER NOT NULL DEFAULT 0,
				created_at      INTEGER NOT NULL,
				updated_at      INTEGER NOT NULL,
				last_seen_at    INTEGER NOT NULL,
				deleted_at      INTEGER
			)`,
			`CREATE INDEX IF NOT EXISTS idx_obs_project   ON observations(project)`,
			`CREATE INDEX IF NOT EXISTS idx_obs_topic_key ON observations(project, scope, topic_key)`,
			`CREATE INDEX IF NOT EXISTS idx_obs_last_seen ON observations(last_seen_at)`,
			// FTS5 virtual table for full-text search
			`CREATE VIRTUAL TABLE IF NOT EXISTS obs_fts USING fts5(
				title, content,
				content='observations', content_rowid='id'
			)`,
			`CREATE TRIGGER IF NOT EXISTS obs_ai AFTER INSERT ON observations BEGIN
				INSERT INTO obs_fts(rowid, title, content) VALUES (new.id, new.title, new.content);
			END`,
			`CREATE TRIGGER IF NOT EXISTS obs_ad AFTER DELETE ON observations BEGIN
				INSERT INTO obs_fts(obs_fts, rowid, title, content) VALUES('delete', old.id, old.title, old.content);
			END`,
			`CREATE TRIGGER IF NOT EXISTS obs_au AFTER UPDATE ON observations BEGIN
				INSERT INTO obs_fts(obs_fts, rowid, title, content) VALUES('delete', old.id, old.title, old.content);
				INSERT INTO obs_fts(rowid, title, content) VALUES (new.id, new.title, new.content);
			END`,
			`CREATE TABLE IF NOT EXISTS sessions (
				id         TEXT    PRIMARY KEY,
				project    TEXT    NOT NULL DEFAULT 'default',
				agent      TEXT    NOT NULL DEFAULT '',
				goal       TEXT    NOT NULL DEFAULT '',
				summary    TEXT    NOT NULL DEFAULT '',
				started_at INTEGER NOT NULL,
				ended_at   INTEGER
			)`,
			`CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions(project)`,
			`CREATE TABLE IF NOT EXISTS prompts (
				id         INTEGER PRIMARY KEY AUTOINCREMENT,
				project    TEXT    NOT NULL DEFAULT 'default',
				content    TEXT    NOT NULL,
				created_at INTEGER NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS attachments (
				id             INTEGER PRIMARY KEY AUTOINCREMENT,
				observation_id INTEGER NOT NULL,
				name           TEXT    NOT NULL,
				mime           TEXT    NOT NULL DEFAULT 'application/octet-stream',
				size_bytes     INTEGER NOT NULL DEFAULT 0,
				sha256         TEXT    NOT NULL DEFAULT '',
				data           BLOB    NOT NULL,
				created_at     INTEGER NOT NULL,
				FOREIGN KEY(observation_id) REFERENCES observations(id) ON DELETE CASCADE
			)`,
			`CREATE INDEX IF NOT EXISTS idx_attachments_observation ON attachments(observation_id)`,
			`CREATE TABLE IF NOT EXISTS observation_content (
				observation_id INTEGER PRIMARY KEY,
				content        TEXT    NOT NULL,
				created_at     INTEGER NOT NULL,
				updated_at     INTEGER NOT NULL,
				FOREIGN KEY(observation_id) REFERENCES observations(id) ON DELETE CASCADE
			)`,
			`CREATE TABLE IF NOT EXISTS observation_relations (
				id         INTEGER PRIMARY KEY AUTOINCREMENT,
				from_id    INTEGER NOT NULL,
				to_id      INTEGER NOT NULL,
				type       TEXT    NOT NULL DEFAULT 'related',
				created_at INTEGER NOT NULL,
				UNIQUE(from_id, to_id, type),
				FOREIGN KEY(from_id) REFERENCES observations(id) ON DELETE CASCADE,
				FOREIGN KEY(to_id) REFERENCES observations(id) ON DELETE CASCADE
			)`,
			`CREATE INDEX IF NOT EXISTS idx_observation_relations_from ON observation_relations(from_id)`,
			`CREATE INDEX IF NOT EXISTS idx_observation_relations_to ON observation_relations(to_id)`,
			`CREATE TABLE IF NOT EXISTS api_keys (
				id         TEXT    PRIMARY KEY,
				name       TEXT    NOT NULL,
				key_hash   TEXT    NOT NULL UNIQUE,
				scopes     TEXT    NOT NULL DEFAULT 'read,write',
				created_at INTEGER NOT NULL,
				expires_at INTEGER,
				revoked    INTEGER NOT NULL DEFAULT 0
			)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash)`,
		}
	}

	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("v1_init: %w\nSQL: %.120s", err, stmt)
		}
	}
	return nil
}

// v2Pgvector adds vector similarity search support (Postgres only).
func (s *Store) v2Pgvector(ctx context.Context, tx *sql.Tx) error {
	stmts := []string{
		// Require the pgvector extension (must be pre-installed on the PG server).
		`CREATE EXTENSION IF NOT EXISTS vector`,
		// Add embedding column — only one dimension per DB instance.
		// ALTER TABLE … ADD COLUMN IF NOT EXISTS is idempotent.
		fmt.Sprintf(
			`ALTER TABLE observations ADD COLUMN IF NOT EXISTS embedding vector(%d)`,
			s.embedDims,
		),
		// IVFFlat index for approximate cosine similarity search.
		// lists=100 is a reasonable default for <1M rows. Tune as needed.
		`CREATE INDEX IF NOT EXISTS idx_obs_embedding ON observations
			USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100)
			WHERE embedding IS NOT NULL`,
	}
	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("v2_pgvector: %w\nSQL: %.200s", err, stmt)
		}
	}
	return nil
}

// v3ContentSplit moves heavy observation content into its own table and trims the inline copy.
func (s *Store) v3ContentSplit(ctx context.Context, tx *sql.Tx) error {
	var stmts []string
	if s.pg() {
		stmts = []string{
			`CREATE TABLE IF NOT EXISTS observation_content (
				observation_id BIGINT      PRIMARY KEY REFERENCES observations(id) ON DELETE CASCADE,
				content        TEXT        NOT NULL,
				created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
			)`,
			`INSERT INTO observation_content(observation_id, content, created_at, updated_at)
			 SELECT id, content, created_at, updated_at
			 FROM observations
			 ON CONFLICT (observation_id) DO NOTHING`,
			`UPDATE observations SET content = LEFT(content, 300) WHERE content <> LEFT(content, 300)`,
		}
	} else {
		stmts = []string{
			`CREATE TABLE IF NOT EXISTS observation_content (
				observation_id INTEGER PRIMARY KEY,
				content        TEXT    NOT NULL,
				created_at     INTEGER NOT NULL,
				updated_at     INTEGER NOT NULL,
				FOREIGN KEY(observation_id) REFERENCES observations(id) ON DELETE CASCADE
			)`,
			`INSERT OR IGNORE INTO observation_content(observation_id, content, created_at, updated_at)
			 SELECT id, content, created_at, updated_at FROM observations`,
			`UPDATE observations SET content = substr(content, 1, 300) WHERE content <> substr(content, 1, 300)`,
		}
	}
	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("v3_content_split: %w\nSQL: %.200s", err, stmt)
		}
	}
	return nil
}

// v4RelationsAndAttachments guarantees the richer Postgres schema exists on older databases.
func (s *Store) v4RelationsAndAttachments(ctx context.Context, tx *sql.Tx) error {
	if !s.pg() {
		return nil
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS attachments (
			id             BIGSERIAL   PRIMARY KEY,
			observation_id BIGINT      NOT NULL REFERENCES observations(id) ON DELETE CASCADE,
			name           TEXT        NOT NULL,
			mime           TEXT        NOT NULL DEFAULT 'application/octet-stream',
			size_bytes     BIGINT      NOT NULL DEFAULT 0,
			sha256         TEXT        NOT NULL DEFAULT '',
			data           BYTEA       NOT NULL,
			created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_attachments_observation ON attachments(observation_id)`,
		`CREATE TABLE IF NOT EXISTS observation_relations (
			id         BIGSERIAL   PRIMARY KEY,
			from_id    BIGINT      NOT NULL REFERENCES observations(id) ON DELETE CASCADE,
			to_id      BIGINT      NOT NULL REFERENCES observations(id) ON DELETE CASCADE,
			type       TEXT        NOT NULL DEFAULT 'related',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(from_id, to_id, type)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_observation_relations_from ON observation_relations(from_id)`,
		`CREATE INDEX IF NOT EXISTS idx_observation_relations_to ON observation_relations(to_id)`,
	}
	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("v4_relations_and_attachments: %w\nSQL: %.200s", err, stmt)
		}
	}
	return nil
}

// v5RepairTextEncoding rebuilds observation previews from the canonical content table.
// This avoids byte-level truncation artifacts in observations.content.
func (s *Store) v5RepairTextEncoding(ctx context.Context, tx *sql.Tx) error {
	if !s.pg() {
		return nil
	}
	_, err := tx.ExecContext(ctx, `
		UPDATE observations o
		   SET content = LEFT(COALESCE(oc.content, o.content), 300)
		  FROM observation_content oc
		 WHERE oc.observation_id = o.id`)
	if err != nil {
		return fmt.Errorf("rebuild observation previews: %w", err)
	}
	return nil
}
