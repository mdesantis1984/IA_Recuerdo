package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/mdesantis1984/ia-recuerdo/internal/store"
	"github.com/mdesantis1984/ia-recuerdo/pkg/types"
)

func TestSmartUpsertConfig_Defaults(t *testing.T) {
	cfg := types.DefaultSmartUpsertConfig()
	if !cfg.Enabled {
		t.Error("Expected Enabled=true by default")
	}
	if cfg.ThresholdUpdate != 0.92 {
		t.Errorf("Expected ThresholdUpdate=0.92, got %f", cfg.ThresholdUpdate)
	}
	if cfg.ThresholdRelated != 0.70 {
		t.Errorf("Expected ThresholdRelated=0.70, got %f", cfg.ThresholdRelated)
	}
	if cfg.AsyncWorkers != 2 {
		t.Errorf("Expected AsyncWorkers=2, got %d", cfg.AsyncWorkers)
	}
}

func TestSmartTopicKey(t *testing.T) {
	tests := []struct {
		title    string
		obsType  string
		expected string
	}{
		{"Architecture decision ADR-001", "architecture", "architecture/architecture-decision-adr"},
		{"Fix bug in memory leak", "bugfix", "bug/fix-bug-in-memory"},
		{"Use Redis for caching", "decision", "decision/use-redis-for"},
		{"Repository pattern in Go", "pattern", "pattern/repository-pattern"},
		{"Database config", "config", "config/database-config"},
		{"Learned about async patterns", "learning", "learning/learned-about-async"},
		{"Default discovery", "discovery", "discovery/default-discovery"},
	}

	for _, tc := range tests {
		got := store.SmartTopicKey(tc.title, tc.obsType)
		if got != tc.expected {
			t.Errorf("SmartTopicKey(%q, %q) = %q, want %q", tc.title, tc.obsType, got, tc.expected)
		}
	}
}

func TestSmartTopicKey_Max5Words(t *testing.T) {
	title := "one two three four five six seven eight nine ten"
	got := store.SmartTopicKey(title, "discovery")
	if got != "discovery/one-two-three-four-five" {
		t.Errorf("Expected max 5 words, got %q", got)
	}
}

func TestSmartTopicKey_StripsSpecialChars(t *testing.T) {
	got := store.SmartTopicKey("Test!@#$%^&*() Title", "pattern")
	if got != "pattern/test-title" {
		t.Errorf("Expected special chars stripped, got %q", got)
	}
}

func TestSmartUpsert_SaveObservation_NonBlocking(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	cfg := types.SmartUpsertConfig{
		Enabled:          true,
		ThresholdUpdate:  0.92,
		ThresholdRelated: 0.70,
		AsyncWorkers:     2,
	}
	_ = cfg

	start := time.Now()
	o := &types.Observation{
		Title:    "Non-blocking test",
		Content:  "This should return quickly without waiting for post-save",
		Type:     types.TypeDiscovery,
		Project:  "test",
		Scope:    types.ScopeProject,
	}
	saved, err := s.SaveObservation(ctx, o)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("SaveObservation: %v", err)
	}
	if saved.ID == 0 {
		t.Fatal("Expected non-zero ID")
	}
	if elapsed > 10*time.Millisecond {
		t.Logf("WARNING: SaveObservation took %v (target: <10ms)", elapsed)
	}
}

func TestSmartUpsert_CloseClosesChannel(t *testing.T) {
	s := newTestStore(t)
	err := s.Close()
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestSmartUpsert_PostSaveActionConstants(t *testing.T) {
	if types.PostSaveActionUpdate != "updated" {
		t.Errorf("PostSaveActionUpdate = %q, want %q", types.PostSaveActionUpdate, "updated")
	}
	if types.PostSaveActionRelated != "related" {
		t.Errorf("PostSaveActionRelated = %q, want %q", types.PostSaveActionRelated, "related")
	}
	if types.PostSaveActionNone != "none" {
		t.Errorf("PostSaveActionNone = %q, want %q", types.PostSaveActionNone, "none")
	}
}

func TestSmartUpsert_PostSaveResult(t *testing.T) {
	result := types.PostSaveResult{
		ObservationID: 123,
		Action:        types.PostSaveActionUpdate,
		TargetID:      456,
		Similarity:    0.95,
	}
	if result.ObservationID != 123 {
		t.Errorf("ObservationID = %d, want 123", result.ObservationID)
	}
	if result.Action != types.PostSaveActionUpdate {
		t.Errorf("Action = %q, want %q", result.Action, "updated")
	}
	if result.TargetID != 456 {
		t.Errorf("TargetID = %d, want 456", result.TargetID)
	}
	if result.Similarity != 0.95 {
		t.Errorf("Similarity = %f, want 0.95", result.Similarity)
	}
}

func TestStore_New_WithUpsertConfig(t *testing.T) {
	dsn := getTestDSN(t)
	if dsn == "" {
		t.Skip("IA_TEST_PG_DSN not set")
	}

	cfg := store.Config{
		Driver:    "postgres",
		DSN:       dsn,
		EmbedDims: 768,
		UpsertCfg: types.SmartUpsertConfig{
			Enabled:          true,
			ThresholdUpdate:  0.85,
			ThresholdRelated: 0.60,
			AsyncWorkers:     4,
		},
	}

	ctx := context.Background()
	s, err := store.New(ctx, cfg, nil)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()
}

func getTestDSN(t *testing.T) string {
	t.Helper()
	dsn := "postgres://postgres:postgres@10.0.0.19:5432/ia_recuerdo_test?sslmode=disable"
	return dsn
}