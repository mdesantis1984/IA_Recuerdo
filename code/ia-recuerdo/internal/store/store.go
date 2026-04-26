// Package store handles all persistence: PostgreSQL + FTS + pgvector.
package store

import (
	"context"
	"database/sql"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mdesantis1984/ia-recuerdo/pkg/types"
)

// Store is the central persistence layer.
type Store struct {
	db        *sql.DB
	driver    string
	embedDims int    // dimension for pgvector column (default 768)
}

// Config holds the database configuration.
type Config struct {
	Driver    string // postgres
	DSN       string // postgres DSN
	EmbedDims int    // vector dimension for pgvector (default 768). Only used on Postgres.
}

// New opens the DB, runs migrations, and returns a ready Store.
func New(ctx context.Context, cfg Config) (*Store, error) {
	db, err := openDB(cfg)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db, driver: strings.ToLower(cfg.Driver), embedDims: cfg.EmbedDims}
	if s.embedDims == 0 {
		s.embedDims = 768
	}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("migration: %w", err)
	}
	log.Printf("[store] Ready (%s)", cfg.Driver)
	return s, nil
}

// Close releases the database connection.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) pg() bool { return true }

// placeholder helper: $N for postgres
func (s *Store) ph(n int) string {
	return fmt.Sprintf("$%d", n)
}

// ────────────────────────────────────────────────────────────────
// Observations
// ────────────────────────────────────────────────────────────────

// SaveObservation inserts or upserts (when topic_key set) an observation.
// Returns the saved observation with its assigned ID.
func (s *Store) SaveObservation(ctx context.Context, o *types.Observation) (*types.Observation, error) {
	now := time.Now().UTC()

	// Upsert by topic_key + project + scope
	if o.TopicKey != "" {
		existing, err := s.findByTopicKey(ctx, o.TopicKey, o.Project, string(o.Scope))
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return s.updateObservation(ctx, existing.ID, o.Title, o.Content, now)
		}
	}

	// Exact dedupe (same title+content+project+type in last 24h)
	if dup, err := s.findDuplicate(ctx, o); err == nil && dup != nil {
		return s.touchDuplicate(ctx, dup.ID, now)
	}

	return s.insertObservation(ctx, o, now)
}

func (s *Store) insertObservation(ctx context.Context, o *types.Observation, now time.Time) (*types.Observation, error) {
	tags := strings.Join(o.Tags, ",")
	scope := string(o.Scope)
	if scope == "" {
		scope = "project"
	}

	var id int64
	var q string
	if s.pg() {
		q = `INSERT INTO observations
			(title, content, type, project, scope, topic_key, tags, session_id,
			 duplicate_count, revision_count, created_at, updated_at, last_seen_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,0,0,$9,$10,$11)
			RETURNING id`
		err := s.db.QueryRowContext(ctx, q,
			o.Title, previewContent(o.Content), string(o.Type), o.Project, scope,
			o.TopicKey, tags, o.SessionID, now, now, now,
		).Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("insert observation: %w", err)
		}
		if err := s.saveObservationContent(ctx, id, o.Content, now); err != nil {
			return nil, err
		}
	} else {
		q = `INSERT INTO observations
			(title, content, type, project, scope, topic_key, tags, session_id,
			 duplicate_count, revision_count, created_at, updated_at, last_seen_at)
			VALUES (?,?,?,?,?,?,?,?,0,0,?,?,?)`
		res, err := s.db.ExecContext(ctx, q,
			o.Title, previewContent(o.Content), string(o.Type), o.Project, scope,
			o.TopicKey, tags, o.SessionID, now.Unix(), now.Unix(), now.Unix(),
		)
		if err != nil {
			return nil, fmt.Errorf("insert observation: %w", err)
		}
		id, _ = res.LastInsertId()
		if err := s.saveObservationContent(ctx, id, o.Content, now); err != nil {
			return nil, err
		}
	}

	o.ID = id
	o.CreatedAt = now
	o.UpdatedAt = now
	o.LastSeenAt = now
	return o, nil
}

func (s *Store) updateObservation(ctx context.Context, id int64, title, content string, now time.Time) (*types.Observation, error) {
	var q string
	if s.pg() {
		// COALESCE(NULLIF(val,''), col) keeps the existing value when the caller passes an empty string.
		q = `UPDATE observations
			 SET title=COALESCE(NULLIF($1,''), title),
			     content=COALESCE(NULLIF($2,''), content),
			     updated_at=$3, last_seen_at=$4,
			     revision_count=revision_count+1
			 WHERE id=$5`
		_, err := s.db.ExecContext(ctx, q, title, content, now, now, id)
		if err != nil {
			return nil, err
		}
		if content != "" {
			if err := s.saveObservationContent(ctx, id, content, now); err != nil {
				return nil, err
			}
		}
	} else {
		q = `UPDATE observations
			 SET title=COALESCE(NULLIF(?,''), title),
			     content=COALESCE(NULLIF(?,''), content),
			     updated_at=?, last_seen_at=?,
			     revision_count=revision_count+1
			 WHERE id=?`
		_, err := s.db.ExecContext(ctx, q, title, content, now.Unix(), now.Unix(), id)
		if err != nil {
			return nil, err
		}
		if content != "" {
			if err := s.saveObservationContent(ctx, id, content, now); err != nil {
				return nil, err
			}
		}
	}
	return s.GetObservation(ctx, id)
}

func (s *Store) touchDuplicate(ctx context.Context, id int64, now time.Time) (*types.Observation, error) {
	var q string
	if s.pg() {
		q = `UPDATE observations SET duplicate_count=duplicate_count+1, last_seen_at=$1 WHERE id=$2`
		_, err := s.db.ExecContext(ctx, q, now, id)
		if err != nil {
			return nil, err
		}
	} else {
		q = `UPDATE observations SET duplicate_count=duplicate_count+1, last_seen_at=? WHERE id=?`
		_, err := s.db.ExecContext(ctx, q, now.Unix(), id)
		if err != nil {
			return nil, err
		}
	}
	return s.GetObservation(ctx, id)
}

// UpdateObservation updates title/content by ID.
func (s *Store) UpdateObservation(ctx context.Context, id int64, title, content string) (*types.Observation, error) {
	return s.updateObservation(ctx, id, title, content, time.Now().UTC())
}

// DeleteObservation soft-deletes an observation (sets deleted_at).
func (s *Store) DeleteObservation(ctx context.Context, id int64, hard bool) error {
	if hard {
		_, err := s.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM observations WHERE id=%s", s.ph(1)), id)
		return err
	}
	now := time.Now().UTC()
	if s.pg() {
		_, err := s.db.ExecContext(ctx, "UPDATE observations SET deleted_at=$1 WHERE id=$2", now, id)
		return err
	}
	_, err := s.db.ExecContext(ctx, "UPDATE observations SET deleted_at=? WHERE id=?", now.Unix(), id)
	return err
}

// GetObservation retrieves a single observation by ID (including soft-deleted).
func (s *Store) GetObservation(ctx context.Context, id int64) (*types.Observation, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+obsColumns()+" FROM observations WHERE id="+s.ph(1), id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list, err := s.scanObservations(rows)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	if content, cerr := s.loadObservationContent(ctx, id); cerr == nil && content != "" {
		list[0].Content = content
	}
	return &list[0], nil
}

// ────────────────────────────────────────────────────────────────
// Search
// ────────────────────────────────────────────────────────────────

// Search performs full-text search using PostgreSQL.
func (s *Store) Search(ctx context.Context, query, project string, limit int) ([]types.SearchResult, error) {
	return s.SearchFiltered(ctx, query, project, nil, limit)
}

// SearchFiltered performs full-text search with optional tag filters.
func (s *Store) SearchFiltered(ctx context.Context, query, project string, tags []string, limit int) ([]types.SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	return s.searchPostgres(ctx, query, project, tags, limit)
}

func (s *Store) searchPostgres(ctx context.Context, query, project string, tags []string, limit int) ([]types.SearchResult, error) {
	var args []interface{}
	where := "deleted_at IS NULL"
	ph := 1
	contentExpr := "COALESCE(oc.content, observations.content)"

	// Full-text search (skipped when query is empty — returns recents by project)
	rankExpr := "0.0 AS rank"
	orderBy := "last_seen_at DESC"
	if query != "" {
		tsquery := strings.Join(strings.Fields(query), " & ")
		where += fmt.Sprintf(" AND to_tsvector('english', title||' '||%s) @@ to_tsquery('english', $%d)", contentExpr, ph)
		args = append(args, tsquery)
		rankExpr = fmt.Sprintf("ts_rank(to_tsvector('english', title||' '||%s), to_tsquery('english', $1)) AS rank", contentExpr)
		orderBy = "rank DESC"
		ph++
	}

	if project != "" {
		where += fmt.Sprintf(" AND project=$%d", ph)
		args = append(args, project)
		ph++
	}
	where, args, ph = appendTagFilters(where, args, tags, ph)

	args = append(args, limit)
	q := fmt.Sprintf(`SELECT %s,
		%s,
		left(%s, 300) AS snippet
		FROM observations
		LEFT JOIN observation_content oc ON oc.observation_id = observations.id WHERE %s
		ORDER BY %s LIMIT $%d`, obsColumns(), rankExpr, contentExpr, where, orderBy, ph)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("search postgres: %w", err)
	}
	defer rows.Close()
	return s.scanResults(rows)
}

// ────────────────────────────────────────────────────────────────
// Context / Timeline
// ────────────────────────────────────────────────────────────────

// RecentContext returns the most recent observations for a project (for session context injection).
func (s *Store) RecentContext(ctx context.Context, project string, limit int) ([]types.Observation, error) {
	return s.RecentContextFiltered(ctx, project, nil, limit)
}

// RecentContextFiltered returns recent observations with optional tag filters.
func (s *Store) RecentContextFiltered(ctx context.Context, project string, tags []string, limit int) ([]types.Observation, error) {
	if limit <= 0 {
		limit = 20
	}
	var q string
	var args []interface{}
	if s.pg() {
		where := "project=$1 AND deleted_at IS NULL"
		args = []interface{}{project}
		where, args, _ = appendTagFilters(where, args, tags, 2)
		q = fmt.Sprintf("SELECT %s FROM observations WHERE %s ORDER BY last_seen_at DESC LIMIT $%d", obsColumns(), where, len(args)+1)
		args = append(args, limit)
	} else {
		q = fmt.Sprintf("SELECT %s FROM observations WHERE project=? AND deleted_at IS NULL ORDER BY last_seen_at DESC LIMIT ?", obsColumns())
		args = []interface{}{project, limit}
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list, err := s.scanObservations(rows)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if content, cerr := s.loadObservationContent(ctx, list[i].ID); cerr == nil && content != "" {
			list[i].Content = content
		}
	}
	return list, nil
}

// MergeProjects reassigns observations, sessions and prompts from one project to another.
func (s *Store) MergeProjects(ctx context.Context, fromProject, toProject string) (int64, error) {
	fromProject = strings.TrimSpace(fromProject)
	toProject = strings.TrimSpace(toProject)
	if fromProject == "" || toProject == "" {
		return 0, fmt.Errorf("fromProject and toProject are required")
	}
	if fromProject == toProject {
		return 0, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	queries := []struct {
		query string
		args  []interface{}
	}{
		{query: "UPDATE observations SET project=$1 WHERE project=$2", args: []interface{}{toProject, fromProject}},
		{query: "UPDATE sessions SET project=$1 WHERE project=$2", args: []interface{}{toProject, fromProject}},
		{query: "UPDATE prompts SET project=$1 WHERE project=$2", args: []interface{}{toProject, fromProject}},
	}

	var affected int64
	for _, item := range queries {
		res, err := tx.ExecContext(ctx, item.query, item.args...)
		if err != nil {
			return 0, err
		}
		if n, err := res.RowsAffected(); err == nil {
			affected += n
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return affected, nil
}

// Timeline returns observations around a given ID (same project, chronological).
func (s *Store) Timeline(ctx context.Context, id int64, window int) ([]types.Observation, error) {
	if window <= 0 {
		window = 5
	}
	ref, err := s.GetObservation(ctx, id)
	if err != nil || ref == nil {
		return nil, err
	}
	var q string
	var args []interface{}
	if s.pg() {
		q = fmt.Sprintf(`SELECT %s FROM observations
			WHERE project=$1 AND deleted_at IS NULL
			AND id BETWEEN $2 AND $3
			ORDER BY id`, obsColumns())
		args = []interface{}{ref.Project, id - int64(window), id + int64(window)}
	} else {
		q = fmt.Sprintf(`SELECT %s FROM observations
			WHERE project=? AND deleted_at IS NULL
			AND id BETWEEN ? AND ?
			ORDER BY id`, obsColumns())
		args = []interface{}{ref.Project, id - int64(window), id + int64(window)}
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanObservations(rows)
}

// ────────────────────────────────────────────────────────────────
// Sessions
// ────────────────────────────────────────────────────────────────

// StartSession creates a new session record and returns its ID.
func (s *Store) StartSession(ctx context.Context, project, agent, goal string) (string, error) {
	id := uuid.New().String()
	now := time.Now().UTC()
	if s.pg() {
		_, err := s.db.ExecContext(ctx,
			"INSERT INTO sessions(id,project,agent,goal,started_at) VALUES($1,$2,$3,$4,$5)",
			id, project, agent, goal, now)
		return id, err
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO sessions(id,project,agent,goal,started_at) VALUES(?,?,?,?,?)",
		id, project, agent, goal, now.Unix())
	return id, err
}

// EndSession marks a session as complete with a summary.
func (s *Store) EndSession(ctx context.Context, id, summary string) error {
	now := time.Now().UTC()
	if s.pg() {
		_, err := s.db.ExecContext(ctx,
			"UPDATE sessions SET ended_at=$1, summary=$2 WHERE id=$3", now, summary, id)
		return err
	}
	_, err := s.db.ExecContext(ctx,
		"UPDATE sessions SET ended_at=?, summary=? WHERE id=?", now.Unix(), summary, id)
	return err
}

// ────────────────────────────────────────────────────────────────
// Prompts
// ────────────────────────────────────────────────────────────────

// SavePrompt persists a reusable prompt.
func (s *Store) SavePrompt(ctx context.Context, project, content string) (*types.Prompt, error) {
	now := time.Now().UTC()
	var id int64
	if s.pg() {
		// ON CONFLICT DO NOTHING guards against sequence desync; if conflict, fetch existing row.
		err := s.db.QueryRowContext(ctx,
			`INSERT INTO prompts(project,content,created_at) VALUES($1,$2,$3)
			 ON CONFLICT DO NOTHING
			 RETURNING id`,
			project, content, now).Scan(&id)
		if err == sql.ErrNoRows {
			// Row already existed (exact duplicate); retrieve its id.
			err = s.db.QueryRowContext(ctx,
				"SELECT id FROM prompts WHERE project=$1 AND content=$2 LIMIT 1",
				project, content).Scan(&id)
		}
		if err != nil {
			return nil, err
		}
	} else {
		res, err := s.db.ExecContext(ctx,
			"INSERT INTO prompts(project,content,created_at) VALUES(?,?,?)",
			project, content, now.Unix())
		if err != nil {
			return nil, err
		}
		id, _ = res.LastInsertId()
	}
	return &types.Prompt{ID: id, Project: project, Content: content, CreatedAt: now}, nil
}

// ────────────────────────────────────────────────────────────────
// Stats
// ────────────────────────────────────────────────────────────────

// Stats returns memory system statistics.
func (s *Store) Stats(ctx context.Context) (*types.Stats, error) {
	st := &types.Stats{
		ByProject: map[string]int{},
		ByType:    map[string]int{},
	}
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM observations WHERE deleted_at IS NULL").Scan(&st.TotalObservations)
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sessions").Scan(&st.TotalSessions)
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(DISTINCT project) FROM observations WHERE deleted_at IS NULL").Scan(&st.TotalProjects)
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM prompts").Scan(&st.TotalPrompts)

	rows, err := s.db.QueryContext(ctx, "SELECT project, COUNT(*) FROM observations WHERE deleted_at IS NULL GROUP BY project")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var p string; var c int
			if rows.Scan(&p, &c) == nil {
				st.ByProject[p] = c
			}
		}
	}
	rows2, err := s.db.QueryContext(ctx, "SELECT type, COUNT(*) FROM observations WHERE deleted_at IS NULL GROUP BY type")
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var t string; var c int
			if rows2.Scan(&t, &c) == nil {
				st.ByType[t] = c
			}
		}
	}
	return st, nil
}

// ────────────────────────────────────────────────────────────────
// Export / Import
// ────────────────────────────────────────────────────────────────

// ListAll returns all non-deleted observations (for export).
func (s *Store) ListAll(ctx context.Context) ([]types.Observation, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+obsColumns()+" FROM observations WHERE deleted_at IS NULL ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list, err := s.scanObservations(rows)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if content, cerr := s.loadObservationContent(ctx, list[i].ID); cerr == nil && content != "" {
			list[i].Content = content
		}
	}
	return list, nil
}

// BulkInsert inserts a slice of observations preserving timestamps (for import).
func (s *Store) BulkInsert(ctx context.Context, obs []types.Observation) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i := range obs {
		o := &obs[i]
		tags := strings.Join(o.Tags, ",")
		if s.pg() {
			err = tx.QueryRowContext(ctx, `INSERT INTO observations
				(title,content,type,project,scope,topic_key,tags,session_id,
				 duplicate_count,revision_count,created_at,updated_at,last_seen_at)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
				ON CONFLICT DO NOTHING RETURNING id`,
				o.Title, previewContent(o.Content), string(o.Type), o.Project, string(o.Scope),
				o.TopicKey, tags, o.SessionID,
				o.DuplicateCount, o.RevisionCount,
				o.CreatedAt, o.UpdatedAt, o.LastSeenAt,
			).Scan(&o.ID)
			if err == nil {
				if cerr := s.saveObservationContentTx(ctx, tx, o.ID, o.Content, o.UpdatedAt); cerr != nil {
					err = cerr
				}
			}
		} else {
			_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO observations
				(title,content,type,project,scope,topic_key,tags,session_id,
				 duplicate_count,revision_count,created_at,updated_at,last_seen_at)
				VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				o.Title, previewContent(o.Content), string(o.Type), o.Project, string(o.Scope),
				o.TopicKey, tags, o.SessionID,
				o.DuplicateCount, o.RevisionCount,
				o.CreatedAt.Unix(), o.UpdatedAt.Unix(), o.LastSeenAt.Unix(),
			)
			if err == nil {
				if cerr := s.saveObservationContentTx(ctx, tx, o.ID, o.Content, o.UpdatedAt); cerr != nil {
					err = cerr
				}
			}
		}
		if err != nil {
			return fmt.Errorf("bulk insert obs %d: %w", i, err)
		}
	}
	return tx.Commit()
}

// ────────────────────────────────────────────────────────────────
// Internal helpers
// ────────────────────────────────────────────────────────────────

func (s *Store) findByTopicKey(ctx context.Context, topicKey, project, scope string) (*types.Observation, error) {
	var q string
	var args []interface{}
	if s.pg() {
		q = "SELECT " + obsColumns() + " FROM observations WHERE topic_key=$1 AND project=$2 AND scope=$3 AND deleted_at IS NULL LIMIT 1"
		args = []interface{}{topicKey, project, scope}
	} else {
		q = "SELECT " + obsColumns() + " FROM observations WHERE topic_key=? AND project=? AND scope=? AND deleted_at IS NULL LIMIT 1"
		args = []interface{}{topicKey, project, scope}
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list, err := s.scanObservations(rows)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return &list[0], nil
}

func (s *Store) findDuplicate(ctx context.Context, o *types.Observation) (*types.Observation, error) {
	cutoff := time.Now().UTC().Add(-24 * time.Hour)
	var q string
	var args []interface{}
	if s.pg() {
		q = "SELECT " + obsColumns() + " FROM observations WHERE title=$1 AND project=$2 AND type=$3 AND created_at>$4 AND deleted_at IS NULL LIMIT 1"
		args = []interface{}{o.Title, o.Project, string(o.Type), cutoff}
	} else {
		q = "SELECT " + obsColumns() + " FROM observations WHERE title=? AND project=? AND type=? AND created_at>? AND deleted_at IS NULL LIMIT 1"
		args = []interface{}{o.Title, o.Project, string(o.Type), cutoff.Unix()}
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list, err := s.scanObservations(rows)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return &list[0], nil
}

func obsColumns() string {
	return `id, title, content, type, project, scope, topic_key, tags, session_id,
		duplicate_count, revision_count, created_at, updated_at, last_seen_at, deleted_at`
}

func obsColumnsAlias(alias string) string {
	cols := []string{"id","title","content","type","project","scope","topic_key","tags","session_id",
		"duplicate_count","revision_count","created_at","updated_at","last_seen_at","deleted_at"}
	for i, c := range cols {
		cols[i] = alias + "." + c
	}
	return strings.Join(cols, ", ")
}

// SaveAttachment persists a blob linked to an observation.
func (s *Store) SaveAttachment(ctx context.Context, observationID int64, name, mime string, data []byte) (*types.Attachment, error) {
	if observationID <= 0 {
		return nil, fmt.Errorf("observationID must be > 0")
	}
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if mime == "" {
		mime = "application/octet-stream"
	}
	h := sha256.Sum256(data)
	checksum := hex.EncodeToString(h[:])
	now := time.Now().UTC()

	var id int64
	if s.pg() {
		if err := s.db.QueryRowContext(ctx,
			`INSERT INTO attachments(observation_id,name,mime,size_bytes,sha256,data,created_at)
			 VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
			observationID, name, mime, len(data), checksum, data, now,
		).Scan(&id); err != nil {
			return nil, err
		}
	} else {
		res, err := s.db.ExecContext(ctx,
			`INSERT INTO attachments(observation_id,name,mime,size_bytes,sha256,data,created_at)
			 VALUES(?,?,?,?,?,?,?)`,
			observationID, name, mime, len(data), checksum, data, now.Unix(),
		)
		if err != nil {
			return nil, err
		}
		id, _ = res.LastInsertId()
	}

	return &types.Attachment{ID: id, ObservationID: observationID, Name: name, Mime: mime, SizeBytes: int64(len(data)), SHA256: checksum, Data: data, CreatedAt: now}, nil
}

// ListAttachments returns attachment metadata for an observation.
func (s *Store) ListAttachments(ctx context.Context, observationID int64) ([]types.Attachment, error) {
	var q string
	var args []interface{}
	if s.pg() {
		q = `SELECT id, observation_id, name, mime, size_bytes, sha256, created_at FROM attachments WHERE observation_id=$1 ORDER BY id`
		args = []interface{}{observationID}
	} else {
		q = `SELECT id, observation_id, name, mime, size_bytes, sha256, created_at FROM attachments WHERE observation_id=? ORDER BY id`
		args = []interface{}{observationID}
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.Attachment
	for rows.Next() {
		var a types.Attachment
		if s.pg() {
			if err := rows.Scan(&a.ID, &a.ObservationID, &a.Name, &a.Mime, &a.SizeBytes, &a.SHA256, &a.CreatedAt); err != nil {
				return nil, err
			}
		} else {
			var createdAt int64
			if err := rows.Scan(&a.ID, &a.ObservationID, &a.Name, &a.Mime, &a.SizeBytes, &a.SHA256, &createdAt); err != nil {
				return nil, err
			}
			a.CreatedAt = time.Unix(createdAt, 0).UTC()
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SaveRelation persists a link between two observations.
func (s *Store) SaveRelation(ctx context.Context, fromID, toID int64, relType string) (*types.Relation, error) {
	if fromID <= 0 || toID <= 0 {
		return nil, fmt.Errorf("fromID and toID must be > 0")
	}
	if relType == "" {
		relType = "related"
	}
	now := time.Now().UTC()
	var id int64
	if s.pg() {
		if err := s.db.QueryRowContext(ctx,
			`INSERT INTO observation_relations(from_id,to_id,type,created_at)
			 VALUES($1,$2,$3,$4)
			 ON CONFLICT (from_id, to_id, type) DO UPDATE SET created_at=EXCLUDED.created_at
			 RETURNING id`,
			fromID, toID, relType, now,
		).Scan(&id); err != nil {
			return nil, err
		}
	} else {
		res, err := s.db.ExecContext(ctx,
			`INSERT OR REPLACE INTO observation_relations(from_id,to_id,type,created_at) VALUES(?,?,?,?)`,
			fromID, toID, relType, now.Unix(),
		)
		if err != nil {
			return nil, err
		}
		id, _ = res.LastInsertId()
	}
	return &types.Relation{ID: id, FromID: fromID, ToID: toID, Type: relType, CreatedAt: now}, nil
}

// ListRelations returns all relations for an observation.
func (s *Store) ListRelations(ctx context.Context, observationID int64) ([]types.Relation, error) {
	var q string
	var args []interface{}
	if s.pg() {
		q = `SELECT id, from_id, to_id, type, created_at FROM observation_relations WHERE from_id=$1 OR to_id=$1 ORDER BY id`
		args = []interface{}{observationID}
	} else {
		q = `SELECT id, from_id, to_id, type, created_at FROM observation_relations WHERE from_id=? OR to_id=? ORDER BY id`
		args = []interface{}{observationID, observationID}
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.Relation
	for rows.Next() {
		var r types.Relation
		if s.pg() {
			if err := rows.Scan(&r.ID, &r.FromID, &r.ToID, &r.Type, &r.CreatedAt); err != nil {
				return nil, err
			}
		} else {
			var createdAt int64
			if err := rows.Scan(&r.ID, &r.FromID, &r.ToID, &r.Type, &createdAt); err != nil {
				return nil, err
			}
			r.CreatedAt = time.Unix(createdAt, 0).UTC()
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func appendTagFilters(where string, args []interface{}, tags []string, ph int) (string, []interface{}, int) {
	for _, tag := range normalizeTags(tags) {
		where += fmt.Sprintf(" AND position(',' || lower($%d) || ',' in ',' || lower(tags) || ',') > 0", ph)
		args = append(args, tag)
		ph++
	}
	return where, args, ph
}

func normalizeTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	out := make([]string, 0, len(tags))
	seen := map[string]struct{}{}
	for _, tag := range tags {
		tag = strings.TrimSpace(strings.ToLower(tag))
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func (s *Store) scanObservations(rows *sql.Rows) ([]types.Observation, error) {
	var list []types.Observation
	for rows.Next() {
		o, err := s.scanOneObs(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *o)
	}
	return list, rows.Err()
}

func (s *Store) scanResults(rows *sql.Rows) ([]types.SearchResult, error) {
	var list []types.SearchResult
	for rows.Next() {
		sr := types.SearchResult{}
		// Pass rank and snippet as extra dest so everything is scanned in one call.
		o, err := s.scanOneObs(rows, &sr.Rank, &sr.Snippet)
		if err != nil {
			return nil, err
		}
		sr.Observation = *o
		list = append(list, sr)
	}
	return list, rows.Err()
}

// scanOneObs reads the obsColumns() fields into an Observation.
// extra optionally receives additional SELECT columns (e.g. rank, snippet for search results).
func (s *Store) scanOneObs(rows *sql.Rows, extra ...interface{}) (*types.Observation, error) {
	o := &types.Observation{}
	var tags string
	var sessionID sql.NullString
	var topicKey sql.NullString

	if s.pg() {
		var deletedAt sql.NullTime
		var createdAt, updatedAt, lastSeenAt time.Time
		dest := []interface{}{
			&o.ID, &o.Title, &o.Content, &o.Type, &o.Project, &o.Scope,
			&topicKey, &tags, &sessionID,
			&o.DuplicateCount, &o.RevisionCount,
			&createdAt, &updatedAt, &lastSeenAt, &deletedAt,
		}
		dest = append(dest, extra...)
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		o.CreatedAt = createdAt
		o.UpdatedAt = updatedAt
		o.LastSeenAt = lastSeenAt
		if deletedAt.Valid {
			o.DeletedAt = &deletedAt.Time
		}
	}

	if topicKey.Valid {
		o.TopicKey = topicKey.String
	}
	if sessionID.Valid {
		o.SessionID = sessionID.String
	}
	if tags != "" {
		o.Tags = strings.Split(tags, ",")
	}
	return o, nil
}

func previewContent(content string) string {
	if len(content) > 300 {
		return content[:300]
	}
	return content
}

func (s *Store) saveObservationContent(ctx context.Context, id int64, content string, now time.Time) error {
	return s.saveObservationContentTx(ctx, s.db, id, content, now)
}

func (s *Store) saveObservationContentTx(ctx context.Context, exec interface{ ExecContext(context.Context, string, ...interface{}) (sql.Result, error) }, id int64, content string, now time.Time) error {
	if s.pg() {
		_, err := exec.ExecContext(ctx, `INSERT INTO observation_content(observation_id, content, created_at, updated_at)
			VALUES($1,$2,$3,$4)
			ON CONFLICT (observation_id) DO UPDATE SET content=EXCLUDED.content, updated_at=EXCLUDED.updated_at`, id, content, now, now)
		return err
	}
	_, err := exec.ExecContext(ctx, `INSERT OR REPLACE INTO observation_content(observation_id, content, created_at, updated_at)
		VALUES(?,?,?,?)`, id, content, now.Unix(), now.Unix())
	return err
}

func (s *Store) loadObservationContent(ctx context.Context, id int64) (string, error) {
	var q string
	var args []interface{}
	if s.pg() {
		q = `SELECT content FROM observation_content WHERE observation_id=$1`
		args = []interface{}{id}
	} else {
		q = `SELECT content FROM observation_content WHERE observation_id=?`
		args = []interface{}{id}
	}
	var content string
	if err := s.db.QueryRowContext(ctx, q, args...).Scan(&content); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return content, nil
}

// ────────────────────────────────────────────────────────────────
// pgvector — semantic search
// ────────────────────────────────────────────────────────────────

// StoreEmbedding sets the vector embedding for an existing observation.
// Should be called right after SaveObservation when an embedding provider is configured.
func (s *Store) StoreEmbedding(ctx context.Context, id int64, embedding []float32) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE observations SET embedding = $1::vector WHERE id = $2",
		vectorToString(embedding), id,
	)
	return err
}

// SemanticSearch finds the most similar observations using cosine distance.
// Available on Postgres with pgvector.
func (s *Store) SemanticSearch(ctx context.Context, embedding []float32, project string, limit int) ([]types.SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}

	var args []interface{}
	args = append(args, vectorToString(embedding)) // $1
	where := "deleted_at IS NULL AND embedding IS NOT NULL"
	ph := 2

	if project != "" {
		where += fmt.Sprintf(" AND project=$%d", ph)
		args = append(args, project)
		ph++
	}
	args = append(args, limit)

	q := fmt.Sprintf(`SELECT %s,
		1 - (embedding <=> $1::vector) AS rank,
		left(COALESCE(oc.content, observations.content), 300) AS snippet
		FROM observations
		LEFT JOIN observation_content oc ON oc.observation_id = observations.id
		WHERE %s
		ORDER BY embedding <=> $1::vector
		LIMIT $%d`, obsColumns(), where, ph)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("semantic search: %w", err)
	}
	defer rows.Close()
	return s.scanResults(rows)
}

// vectorToString formats a []float32 as a Postgres vector literal "[0.1,0.2,...]".
func vectorToString(v []float32) string {
	if len(v) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%g", f)
	}
	b.WriteByte(']')
	return b.String()
}
