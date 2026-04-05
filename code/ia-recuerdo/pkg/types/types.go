// Package types defines all domain types for IA_Recuerdo.
package types

import "time"

// ObservationType categorizes a memory.
type ObservationType string

const (
	TypeDecision    ObservationType = "decision"
	TypeBugfix      ObservationType = "bugfix"
	TypePattern     ObservationType = "pattern"
	TypeConfig      ObservationType = "config"
	TypeDiscovery   ObservationType = "discovery"
	TypeLearning    ObservationType = "learning"
	TypeArchitecture ObservationType = "architecture"
)

// Scope controls visibility of a memory.
type Scope string

const (
	ScopeProject  Scope = "project"
	ScopePersonal Scope = "personal"
)

// Observation is the core memory unit for IA_Recuerdo.
type Observation struct {
	ID             int64           `json:"id"`
	Title          string          `json:"title"`
	Content        string          `json:"content"`
	Type           ObservationType `json:"type"`
	Project        string          `json:"project"`
	Scope          Scope           `json:"scope"`
	TopicKey       string          `json:"topic_key,omitempty"`
	Tags           []string        `json:"tags,omitempty"`
	Embedding      []float32       `json:"embedding,omitempty"` // pgvector
	DuplicateCount int             `json:"duplicate_count"`
	RevisionCount  int             `json:"revision_count"`
	SessionID      string          `json:"session_id,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	LastSeenAt     time.Time       `json:"last_seen_at"`
	DeletedAt      *time.Time      `json:"deleted_at,omitempty"`
}

// Session tracks an agent session lifecycle.
type Session struct {
	ID        string     `json:"id"`
	Project   string     `json:"project"`
	Agent     string     `json:"agent,omitempty"` // "vscode", "claude-code", "opencode", etc.
	Goal      string     `json:"goal,omitempty"`
	Summary   string     `json:"summary,omitempty"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}

// Prompt stores a saved user prompt for reuse.
type Prompt struct {
	ID        int64     `json:"id"`
	Project   string    `json:"project"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// SearchResult wraps an Observation with relevance metadata.
type SearchResult struct {
	Observation
	Rank    float64 `json:"rank"`
	Snippet string  `json:"snippet"` // ~100-token preview
}

// Stats holds memory system statistics.
type Stats struct {
	TotalObservations int            `json:"total_observations"`
	TotalSessions     int            `json:"total_sessions"`
	TotalProjects     int            `json:"total_projects"`
	TotalPrompts      int            `json:"total_prompts"`
	ByProject         map[string]int `json:"by_project"`
	ByType            map[string]int `json:"by_type"`
}
