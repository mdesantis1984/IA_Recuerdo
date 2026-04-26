package store_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/mdesantis1984/ia-recuerdo/internal/store"
	"github.com/mdesantis1984/ia-recuerdo/pkg/types"
)

// newTestStore creates a PostgreSQL-backed store for testing.
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dsn := os.Getenv("IA_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("IA_TEST_PG_DSN not set")
	}
	s, err := store.New(context.Background(), store.Config{
		Driver: "postgres",
		DSN:    dsn,
	})
	if err != nil {
		t.Fatalf("newTestStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// ─────────────────────────────────────────────────────────────────
// SaveObservation
// ─────────────────────────────────────────────────────────────────

func TestSaveObservation_Basic(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	o := &types.Observation{
		Title:   "First memory",
		Content: "Something important happened",
		Type:    types.TypeDiscovery,
		Project: "test",
	}
	saved, err := s.SaveObservation(ctx, o)
	if err != nil {
		t.Fatalf("SaveObservation: %v", err)
	}
	if saved.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if saved.CreatedAt.IsZero() {
		t.Fatal("expected non-zero CreatedAt")
	}
}

func TestSaveObservation_UpsertByTopicKey(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	o := &types.Observation{
		Title:    "Architecture decision",
		Content:  "We chose PostgreSQL",
		Type:     types.TypeDecision,
		Project:  "test",
		Scope:    types.ScopeProject,
		TopicKey: "architecture/database-choice",
	}
	first, err := s.SaveObservation(ctx, o)
	if err != nil {
		t.Fatalf("first save: %v", err)
	}

	// Update via same topic_key
	o2 := &types.Observation{
		Title:    "Architecture decision",
		Content:  "We chose PostgreSQL — confirmed after benchmarks",
		Type:     types.TypeDecision,
		Project:  "test",
		Scope:    types.ScopeProject,
		TopicKey: "architecture/database-choice",
	}
	second, err := s.SaveObservation(ctx, o2)
	if err != nil {
		t.Fatalf("upsert save: %v", err)
	}

	if first.ID != second.ID {
		t.Fatalf("upsert should reuse same ID, got %d vs %d", first.ID, second.ID)
	}
	if second.RevisionCount != 1 {
		t.Fatalf("expected revision_count=1, got %d", second.RevisionCount)
	}
}

func TestSaveObservation_DedupeSameTitle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	o := &types.Observation{
		Title:   "Same title",
		Content: "Same content",
		Type:    types.TypeDiscovery,
		Project: "test",
	}
	first, _ := s.SaveObservation(ctx, o)
	second, _ := s.SaveObservation(ctx, o) // duplicate within 24h

	if first.ID != second.ID {
		t.Fatalf("dedupe: expected same ID, got %d vs %d", first.ID, second.ID)
	}
	if second.DuplicateCount != 1 {
		t.Fatalf("expected duplicate_count=1, got %d", second.DuplicateCount)
	}
}

// ─────────────────────────────────────────────────────────────────
// GetObservation / UpdateObservation / DeleteObservation
// ─────────────────────────────────────────────────────────────────

func TestGetObservation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	saved, _ := s.SaveObservation(ctx, &types.Observation{
		Title: "Fetchable", Content: "content", Type: types.TypeLearning, Project: "p1",
	})

	got, err := s.GetObservation(ctx, saved.ID)
	if err != nil {
		t.Fatalf("GetObservation: %v", err)
	}
	if got == nil || got.ID != saved.ID {
		t.Fatal("observation not found or ID mismatch")
	}
	if got.Title != "Fetchable" {
		t.Fatalf("title mismatch: %q", got.Title)
	}
	if got.Content != "content" {
		t.Fatalf("expected full content, got %q", got.Content)
	}
}

func TestGetObservation_NotFound(t *testing.T) {
	s := newTestStore(t)
	got, err := s.GetObservation(context.Background(), 99999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for nonexistent ID")
	}
}

func TestUpdateObservation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	saved, _ := s.SaveObservation(ctx, &types.Observation{
		Title: "Old title", Content: "Old content", Type: types.TypeDiscovery, Project: "p",
	})

	updated, err := s.UpdateObservation(ctx, saved.ID, "New title", "New content")
	if err != nil {
		t.Fatalf("UpdateObservation: %v", err)
	}
	if updated.Title != "New title" {
		t.Fatalf("expected %q, got %q", "New title", updated.Title)
	}
	if updated.RevisionCount != 1 {
		t.Fatalf("expected revision_count=1, got %d", updated.RevisionCount)
	}
}

func TestDeleteObservation_Soft(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	saved, _ := s.SaveObservation(ctx, &types.Observation{
		Title: "To delete", Content: "bye", Type: types.TypeDiscovery, Project: "p",
	})

	if err := s.DeleteObservation(ctx, saved.ID, false); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	got, _ := s.GetObservation(ctx, saved.ID)
	if got == nil || got.DeletedAt == nil {
		t.Fatal("expected deleted_at to be set after soft delete")
	}
}

func TestDeleteObservation_Hard(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	saved, _ := s.SaveObservation(ctx, &types.Observation{
		Title: "Hard delete", Content: "gone", Type: types.TypeDiscovery, Project: "p",
	})

	if err := s.DeleteObservation(ctx, saved.ID, true); err != nil {
		t.Fatalf("hard delete: %v", err)
	}

	got, _ := s.GetObservation(ctx, saved.ID)
	if got != nil {
		t.Fatal("expected nil after hard delete")
	}
}

// ─────────────────────────────────────────────────────────────────
// Search
// ─────────────────────────────────────────────────────────────────

func TestSearch_FindsMatch(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	s.SaveObservation(ctx, &types.Observation{
		Title: "Valkey cache setup", Content: "We configured Valkey for caching", Type: types.TypeConfig, Project: "infra",
	})
	s.SaveObservation(ctx, &types.Observation{
		Title: "Postgres migration", Content: "Migrated database to Postgres", Type: types.TypeDecision, Project: "infra",
	})

	results, err := s.Search(ctx, "Valkey", "infra", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result for 'Valkey'")
	}
	if results[0].Title != "Valkey cache setup" {
		t.Fatalf("unexpected top result: %q", results[0].Title)
	}
}

func TestSearch_ReturnsEmptyOnNoMatch(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	s.SaveObservation(ctx, &types.Observation{
		Title: "Something", Content: "Unrelated", Type: types.TypeDiscovery, Project: "p",
	})

	results, err := s.Search(ctx, "zzznomatch", "p", 10)
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestSearch_DefaultLimit(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for i := 0; i < 15; i++ {
		s.SaveObservation(ctx, &types.Observation{
			Title:   fmt.Sprintf("Cache thing %d", i),
			Content: "Valkey cache entry",
			Type:    types.TypeConfig,
			Project: "p",
			Tags:    []string{"cache"},
		})
	}

	results, err := s.Search(ctx, "Valkey", "p", 0) // limit=0 → default 10
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) > 10 {
		t.Fatalf("expected at most 10, got %d", len(results))
	}
}

// ─────────────────────────────────────────────────────────────────
// RecentContext / Timeline
// ─────────────────────────────────────────────────────────────────

func TestRecentContext(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		s.SaveObservation(ctx, &types.Observation{
			Title:   fmt.Sprintf("Distinct obs %d", i),
			Content: fmt.Sprintf("content %d", i),
			Type:    types.TypeDiscovery,
			Project: "proj",
		})
	}

	obs, err := s.RecentContext(ctx, "proj", 3)
	if err != nil {
		t.Fatalf("RecentContext: %v", err)
	}
	if len(obs) != 3 {
		t.Fatalf("expected 3, got %d", len(obs))
	}
}

func TestTimeline(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	var mid int64
	for i := 0; i < 10; i++ {
		o, _ := s.SaveObservation(ctx, &types.Observation{
			Title:   "Timeline obs",
			Content: "content",
			Type:    types.TypeDiscovery,
			Project: "p",
		})
		if i == 4 {
			mid = o.ID
		}
	}

	obs, err := s.Timeline(ctx, mid, 2)
	if err != nil {
		t.Fatalf("Timeline: %v", err)
	}
	if len(obs) == 0 {
		t.Fatal("expected non-empty timeline")
	}
}

// ─────────────────────────────────────────────────────────────────
// Sessions
// ─────────────────────────────────────────────────────────────────

func TestSessionStartEnd(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id, err := s.StartSession(ctx, "project-x", "vscode", "Implement feature Y")
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty session ID")
	}

	if err := s.EndSession(ctx, id, "Feature Y done"); err != nil {
		t.Fatalf("EndSession: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────
// Prompts
// ─────────────────────────────────────────────────────────────────

func TestSavePrompt(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	p, err := s.SavePrompt(ctx, "proj", "You are an expert Go developer.")
	if err != nil {
		t.Fatalf("SavePrompt: %v", err)
	}
	if p.ID == 0 {
		t.Fatal("expected non-zero prompt ID")
	}
}

// ─────────────────────────────────────────────────────────────────
// Stats
// ─────────────────────────────────────────────────────────────────

func TestStats(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	s.SaveObservation(ctx, &types.Observation{Title: "A", Content: "B", Type: types.TypeDiscovery, Project: "p1"})
	s.SaveObservation(ctx, &types.Observation{Title: "C", Content: "D", Type: types.TypeDecision, Project: "p2"})
	s.StartSession(ctx, "p1", "agent", "goal")

	st, err := s.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if st.TotalObservations != 2 {
		t.Fatalf("expected 2 observations, got %d", st.TotalObservations)
	}
	if st.TotalSessions != 1 {
		t.Fatalf("expected 1 session, got %d", st.TotalSessions)
	}
	if st.TotalProjects != 2 {
		t.Fatalf("expected 2 projects, got %d", st.TotalProjects)
	}
}

// ─────────────────────────────────────────────────────────────────
// Export / Import
// ─────────────────────────────────────────────────────────────────

func TestListAll(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	s.SaveObservation(ctx, &types.Observation{Title: "T1", Content: "C1", Type: types.TypeDiscovery, Project: "p"})
	s.SaveObservation(ctx, &types.Observation{Title: "T2", Content: "C2", Type: types.TypeDecision, Project: "p"})

	all, err := s.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2, got %d", len(all))
	}
}

func TestSaveAttachment(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	obs, _ := s.SaveObservation(ctx, &types.Observation{Title: "Attachment parent", Content: "content", Type: types.TypeDiscovery, Project: "p"})
	att, err := s.SaveAttachment(ctx, obs.ID, "note.txt", "text/plain", []byte("hello"))
	if err != nil {
		t.Fatalf("SaveAttachment: %v", err)
	}
	if att.ID == 0 || att.SHA256 == "" {
		t.Fatal("expected attachment to be stored")
	}
	items, err := s.ListAttachments(ctx, obs.ID)
	if err != nil {
		t.Fatalf("ListAttachments: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(items))
	}
}

func TestSaveRelation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	a, _ := s.SaveObservation(ctx, &types.Observation{Title: "A", Content: "content", Type: types.TypeDiscovery, Project: "p"})
	b, _ := s.SaveObservation(ctx, &types.Observation{Title: "B", Content: "content", Type: types.TypeDiscovery, Project: "p"})
	rel, err := s.SaveRelation(ctx, a.ID, b.ID, "related")
	if err != nil {
		t.Fatalf("SaveRelation: %v", err)
	}
	if rel.ID == 0 {
		t.Fatal("expected relation id")
	}
	items, err := s.ListRelations(ctx, a.ID)
	if err != nil {
		t.Fatalf("ListRelations: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 relation, got %d", len(items))
	}
}

func TestBulkInsert(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC()
	batch := []types.Observation{
		{Title: "Imported 1", Content: "imported observation", Type: types.TypeDiscovery, Project: "migrated", Scope: types.ScopeProject, CreatedAt: now, UpdatedAt: now, LastSeenAt: now},
		{Title: "Imported 2", Content: "imported observation", Type: types.TypeDecision, Project: "migrated", Scope: types.ScopeProject, CreatedAt: now, UpdatedAt: now, LastSeenAt: now},
	}
	if err := s.BulkInsert(ctx, batch); err != nil {
		t.Fatalf("BulkInsert: %v", err)
	}

	all, _ := s.ListAll(ctx)
	if len(all) != 2 {
		t.Fatalf("expected 2 after BulkInsert, got %d", len(all))
	}
}

// ─────────────────────────────────────────────────────────────────
// API Keys
// ─────────────────────────────────────────────────────────────────

func TestAPIKeys(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create
	id := "key-001"
	hash := "sha256hashvalue"
	if err := s.CreateAPIKey(ctx, id, "test-agent", hash, "read,write"); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	// Validate
	valid, err := s.ValidateAPIKey(ctx, hash)
	if err != nil {
		t.Fatalf("ValidateAPIKey: %v", err)
	}
	if !valid {
		t.Fatal("expected key to be valid")
	}

	// List
	keys, err := s.ListAPIKeys(ctx)
	if err != nil {
		t.Fatalf("ListAPIKeys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}

	// Revoke
	if err := s.RevokeAPIKey(ctx, id); err != nil {
		t.Fatalf("RevokeAPIKey: %v", err)
	}
	valid, _ = s.ValidateAPIKey(ctx, hash)
	if valid {
		t.Fatal("expected key to be invalid after revoke")
	}
}

func TestValidateAPIKey_InvalidHash(t *testing.T) {
	s := newTestStore(t)
	valid, err := s.ValidateAPIKey(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if valid {
		t.Fatal("expected false for nonexistent key hash")
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
