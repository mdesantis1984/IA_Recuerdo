// Package mcp implements the 16 MCP tools exposed by IA_Recuerdo.
// Tool contracts follow the MCP specification.
package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mdesantis1984/ia-recuerdo/internal/cache"
	"github.com/mdesantis1984/ia-recuerdo/internal/embedding"
	"github.com/mdesantis1984/ia-recuerdo/internal/store"
	"github.com/mdesantis1984/ia-recuerdo/pkg/types"
)

// Handler holds the store reference and handles every MCP tool call.
type Handler struct {
	store    *store.Store
	cache    *cache.Cache
	embedder embedding.Provider
}

// New creates a new MCP Handler. Pass nil for ca or emb to disable those features.
func New(s *store.Store, ca *cache.Cache, emb embedding.Provider) *Handler {
	if ca == nil {
		ca = cache.New("") // no-op cache
	}
	if emb == nil {
		emb = &embedding.Disabled{}
	}
	return &Handler{store: s, cache: ca, embedder: emb}
}

// Call dispatches a tool call by name.
func (h *Handler) Call(ctx context.Context, tool string, args map[string]interface{}) (interface{}, error) {
	switch tool {
	case "mem_save":
		return h.memSave(ctx, args)
	case "mem_update":
		return h.memUpdate(ctx, args)
	case "mem_delete":
		return h.memDelete(ctx, args)
	case "mem_suggest_topic_key":
		return h.memSuggestTopicKey(ctx, args)
	case "mem_search":
		return h.memSearch(ctx, args)
	case "mem_session_summary":
		return h.memSessionSummary(ctx, args)
	case "mem_context":
		return h.memContext(ctx, args)
	case "mem_timeline":
		return h.memTimeline(ctx, args)
	case "mem_get_observation":
		return h.memGetObservation(ctx, args)
	case "mem_save_prompt":
		return h.memSavePrompt(ctx, args)
	case "mem_stats":
		return h.memStats(ctx, args)
	case "mem_session_start":
		return h.memSessionStart(ctx, args)
	case "mem_session_end":
		return h.memSessionEnd(ctx, args)
	case "mem_capture_passive":
		return h.memCapturePassive(ctx, args)
	case "mem_merge_projects":
		return h.memMergeProjects(ctx, args)
	case "mem_save_attachment":
		return h.memSaveAttachment(ctx, args)
	case "mem_list_attachments":
		return h.memListAttachments(ctx, args)
	case "mem_save_relation":
		return h.memSaveRelation(ctx, args)
	case "mem_list_relations":
		return h.memListRelations(ctx, args)
	case "mem_semantic_search":
		return h.memSemanticSearch(ctx, args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", tool)
	}
}

// ToolList returns the MCP tools manifest for mcp.tools.list.
func ToolList() []map[string]interface{} {
	return []map[string]interface{}{
		tool("mem_save", "Save a structured memory observation",
			prop("title", "string", "Short title (5-10 words)"),
			prop("content", "string", "What/Why/Where/Learned format"),
			prop("type", "string", "decision|bugfix|pattern|config|discovery|learning|architecture"),
			prop("project", "string", "Project name"),
			prop("scope", "string", "project (default) or personal"),
			prop("topic_key", "string", "Stable key for upsert (optional)"),
			prop("tags", "array", "Optional string tags"),
			[]string{"title", "content", "type"},
		),
		tool("mem_update", "Update an existing observation by ID",
			prop("id", "integer", "Observation ID"),
			prop("title", "string", "New title"),
			prop("content", "string", "New content"),
			nil,
			[]string{"id"},
		),
		tool("mem_delete", "Soft-delete (default) or hard-delete an observation",
			prop("id", "integer", "Observation ID"),
			prop("hard", "boolean", "Hard delete (irreversible)"),
			nil,
			[]string{"id"},
		),
		tool("mem_suggest_topic_key", "Suggest a stable topic_key before saving",
			prop("type", "string", "Observation type"),
			prop("title", "string", "Observation title"),
			nil,
			[]string{"type", "title"},
		),
		tool("mem_search", "Full-text search across all memories",
			prop("query", "string", "Search query"),
			prop("project", "string", "Filter by project (optional)"),
			prop("tags", "array", "Optional string tags"),
			prop("limit", "integer", "Max results (default 10)"),
			nil,
			[]string{"query"},
		),
		tool("mem_session_summary", "Save end-of-session summary",
			prop("goal", "string", "Session goal"),
			prop("discoveries", "string", "Key discoveries"),
			prop("accomplished", "string", "What was accomplished"),
			prop("files", "string", "Files changed"),
			prop("project", "string", "Project name"),
			prop("session_id", "string", "Session ID from mem_session_start"),
			[]string{"goal"},
		),
		tool("mem_context", "Get recent context from previous sessions",
			prop("project", "string", "Project name"),
			prop("tags", "array", "Optional string tags"),
			prop("limit", "integer", "Max observations (default 20)"),
			nil, nil,
		),
		tool("mem_timeline", "Chronological context around an observation",
			prop("observation_id", "integer", "Observation ID"),
			prop("window", "integer", "Observations before/after (default 5)"),
			nil,
			[]string{"observation_id"},
		),
		tool("mem_get_observation", "Get full untruncated content of an observation",
			prop("id", "integer", "Observation ID"),
			nil,
			[]string{"id"},
		),
		tool("mem_save_prompt", "Save a reusable user prompt",
			prop("content", "string", "Prompt content"),
			prop("project", "string", "Project name"),
			nil,
			[]string{"content"},
		),
		tool("mem_stats", "Memory system statistics", nil, nil, nil),
		tool("mem_session_start", "Register session start",
			prop("project", "string", "Project name"),
			prop("agent", "string", "Agent name (vscode, claude-code, etc.)"),
			prop("goal", "string", "Session goal"),
			nil, nil,
		),
		tool("mem_session_end", "Mark session as complete",
			prop("session_id", "string", "Session ID"),
			prop("summary", "string", "Brief summary"),
			nil,
			[]string{"session_id"},
		),
		tool("mem_capture_passive", "Extract learnings from text output",
			prop("text", "string", "Text to analyze"),
			prop("project", "string", "Project name"),
			nil,
			[]string{"text"},
		),
		tool("mem_merge_projects", "Merge project name variants (admin)",
			prop("from", "string", "Source project name"),
			prop("to", "string", "Target canonical project name"),
			nil,
			[]string{"from", "to"},
		),
		tool("mem_save_attachment", "Save a binary attachment linked to an observation",
			prop("observation_id", "integer", "Observation ID"),
			prop("name", "string", "Attachment name"),
			prop("mime", "string", "MIME type"),
			prop("data_b64", "string", "Base64-encoded content"),
			nil,
			[]string{"observation_id", "name", "data_b64"},
		),
		tool("mem_list_attachments", "List attachments for an observation",
			prop("observation_id", "integer", "Observation ID"),
			nil,
			[]string{"observation_id"},
		),
		tool("mem_save_relation", "Save a relation between two observations",
			prop("from_id", "integer", "Source observation ID"),
			prop("to_id", "integer", "Target observation ID"),
			prop("type", "string", "Relation type"),
			nil,
			[]string{"from_id", "to_id"},
		),
		tool("mem_list_relations", "List relations for an observation",
			prop("observation_id", "integer", "Observation ID"),
			nil,
			[]string{"observation_id"},
		),
		tool("mem_semantic_search", "Semantic similarity search using vector embeddings (requires embedding provider)",
			prop("query", "string", "Natural language query to search by meaning"),
			prop("project", "string", "Filter by project (optional)"),
			prop("limit", "integer", "Max results (default 10)"),
			nil,
			[]string{"query"},
		),
	}
}

// ─────────────────────────────────────────────────────────────────
// Tool implementations
// ─────────────────────────────────────────────────────────────────

func (h *Handler) memSave(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	title := str(args, "title")
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}
	content := str(args, "content")
	if content == "" {
		return nil, fmt.Errorf("content is required")
	}
	o := &types.Observation{
		Title:    title,
		Content:  content,
		Type:     types.ObservationType(strDefault(args, "type", "discovery")),
		Project:  strDefault(args, "project", "default"),
		Scope:    types.Scope(strDefault(args, "scope", "project")),
		TopicKey: str(args, "topic_key"),
	}
	if tags, ok := args["tags"].([]interface{}); ok {
		for _, t := range tags {
			if s, ok := t.(string); ok {
				o.Tags = append(o.Tags, s)
			}
		}
	}
	saved, err := h.store.SaveObservation(ctx, o)
	if err != nil {
		return nil, err
	}
	// Generate and store embedding if a provider is configured.
	// Fire-and-forget: embedding failures don't fail the save.
	if emb, embErr := h.embedder.Embed(ctx, o.Title+" "+o.Content); embErr == nil {
		_ = h.store.StoreEmbedding(ctx, saved.ID, emb)
	}
	// Invalidate context cache after write so mem_context returns fresh data.
	h.cache.InvalidateContext(ctx, o.Project)
	return map[string]interface{}{
		"id":      saved.ID,
		"status":  "saved",
		"message": fmt.Sprintf("Memory saved: %q (%s)", saved.Title, saved.Type),
	}, nil
}

func (h *Handler) memUpdate(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	id := int64(num(args, "id"))
	if id <= 0 {
		return nil, fmt.Errorf("id is required and must be > 0")
	}
	updated, err := h.store.UpdateObservation(ctx, id, str(args, "title"), str(args, "content"))
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, fmt.Errorf("observation %d not found", id)
	}
	return map[string]interface{}{"id": updated.ID, "revision_count": updated.RevisionCount, "status": "updated"}, nil
}

func (h *Handler) memDelete(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	id := int64(num(args, "id"))
	if id <= 0 {
		return nil, fmt.Errorf("id is required and must be > 0")
	}
	hard := boolArg(args, "hard")
	if err := h.store.DeleteObservation(ctx, id, hard); err != nil {
		return nil, err
	}
	return map[string]interface{}{"status": "deleted", "hard": hard}, nil
}

func (h *Handler) memSuggestTopicKey(_ context.Context, args map[string]interface{}) (interface{}, error) {
	t := strings.ToLower(str(args, "type"))
	title := strings.ToLower(str(args, "title"))
	// Normalize title to slug
	words := strings.Fields(title)
	if len(words) > 5 {
		words = words[:5]
	}
	slug := strings.Join(words, "-")
	slug = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, slug)
	family := "discovery"
	switch {
	case strings.Contains(t, "arch"), strings.Contains(t, "design"), strings.Contains(t, "adr"):
		family = "architecture"
	case strings.Contains(t, "bug"), strings.Contains(t, "fix"), strings.Contains(t, "error"):
		family = "bug"
	case strings.Contains(t, "decision"):
		family = "decision"
	case strings.Contains(t, "pattern"):
		family = "pattern"
	case strings.Contains(t, "config"):
		family = "config"
	case strings.Contains(t, "learning"), strings.Contains(t, "lesson"):
		family = "learning"
	}
	key := fmt.Sprintf("%s/%s", family, slug)
	return map[string]interface{}{"topic_key": key}, nil
}

func (h *Handler) memSearch(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	query := str(args, "query")
	project := str(args, "project")
	limit := int(num(args, "limit"))
	tags := stringSlice(args, "tags")

	// Cache lookup (fingerprint: query|project|limit)
	fp := fmt.Sprintf("%s|%s|%d", query, project, limit)
	var cached map[string]interface{}
	if h.cache.GetSearch(ctx, fp, &cached) {
		return cached, nil
	}

	results, err := h.store.SearchFiltered(ctx, query, project, tags, limit)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]interface{}, 0, len(results))
	for _, r := range results {
		out = append(out, map[string]interface{}{
			"id":         r.ID,
			"title":      r.Title,
			"type":       r.Type,
			"project":    r.Project,
			"snippet":    r.Snippet,
			"rank":       r.Rank,
			"created_at": r.CreatedAt,
		})
	}
	resp := map[string]interface{}{"results": out, "count": len(out)}
	h.cache.SetSearch(ctx, fp, resp)
	return resp, nil
}

func (h *Handler) memSessionSummary(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	content := fmt.Sprintf("## Session Summary\n\n**Goal**: %s\n\n**Accomplished**: %s\n\n**Discoveries**: %s\n\n**Files**: %s",
		str(args, "goal"), str(args, "accomplished"), str(args, "discoveries"), str(args, "files"))

	o := &types.Observation{
		Title:     fmt.Sprintf("Session summary — %s", str(args, "goal")),
		Content:   content,
		Type:      types.TypeLearning,
		Project:   strDefault(args, "project", "default"),
		Scope:     types.ScopeProject,
		SessionID: str(args, "session_id"),
	}
	saved, err := h.store.SaveObservation(ctx, o)
	if err != nil {
		return nil, err
	}
	if sid := str(args, "session_id"); sid != "" {
		_ = h.store.EndSession(ctx, sid, str(args, "accomplished"))
	}
	return map[string]interface{}{"id": saved.ID, "status": "summary_saved"}, nil
}

func (h *Handler) memContext(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	limit := int(num(args, "limit"))
	if limit == 0 {
		limit = 20
	}
	project := strDefault(args, "project", "default")
	tags := stringSlice(args, "tags")

	// Cache lookup (TTL 1h, invalidated on mem_save)
	var cached map[string]interface{}
	if h.cache.GetContext(ctx, project, &cached) {
		return cached, nil
	}

	obs, err := h.store.RecentContextFiltered(ctx, project, tags, limit)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]interface{}, 0, len(obs))
	for _, o := range obs {
		snippet := o.Content
		if len(snippet) > 300 {
			snippet = snippet[:300] + "..."
		}
		out = append(out, map[string]interface{}{
			"id": o.ID, "title": o.Title, "type": o.Type,
			"snippet": snippet, "last_seen_at": o.LastSeenAt,
		})
	}
	resp := map[string]interface{}{"observations": out, "count": len(out)}
	h.cache.SetContext(ctx, project, resp)
	return resp, nil
}

func (h *Handler) memTimeline(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	id := int64(num(args, "observation_id"))
	if id <= 0 {
		return nil, fmt.Errorf("observation_id is required and must be > 0")
	}
	obs, err := h.store.Timeline(ctx, id, int(num(args, "window")))
	if err != nil {
		return nil, err
	}
	out := obs
	if out == nil {
		out = []types.Observation{}
	}
	return map[string]interface{}{"observations": out, "count": len(out)}, nil
}

func (h *Handler) memGetObservation(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	id := int64(num(args, "id"))
	if id <= 0 {
		return nil, fmt.Errorf("id is required and must be > 0")
	}
	o, err := h.store.GetObservation(ctx, id)
	if err != nil {
		return nil, err
	}
	if o == nil {
		return nil, fmt.Errorf("observation %d not found", id)
	}
	return o, nil
}

func (h *Handler) memSavePrompt(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	p, err := h.store.SavePrompt(ctx, strDefault(args, "project", "default"), str(args, "content"))
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"id": p.ID, "status": "prompt_saved"}, nil
}

func (h *Handler) memStats(ctx context.Context, _ map[string]interface{}) (interface{}, error) {
	return h.store.Stats(ctx)
}

func (h *Handler) memSessionStart(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	id, err := h.store.StartSession(ctx,
		strDefault(args, "project", "default"),
		strDefault(args, "agent", "unknown"),
		str(args, "goal"),
	)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"session_id": id, "status": "session_started"}, nil
}

func (h *Handler) memSessionEnd(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sid := str(args, "session_id")
	if sid == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if err := h.store.EndSession(ctx, sid, str(args, "summary")); err != nil {
		return nil, err
	}
	return map[string]interface{}{"status": "session_ended"}, nil
}

func (h *Handler) memCapturePassive(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	text := str(args, "text")
	if len(text) < 50 {
		return map[string]interface{}{"status": "skipped", "reason": "text too short"}, nil
	}
	// Heuristic: save as a discovery with text truncated to 1000 chars
	if len(text) > 1000 {
		text = text[:1000] + "..."
	}
	o := &types.Observation{
		Title:   "Passive capture — " + text[:min(60, len(text))],
		Content: text,
		Type:    types.TypeDiscovery,
		Project: strDefault(args, "project", "default"),
		Scope:   types.ScopeProject,
	}
	saved, err := h.store.SaveObservation(ctx, o)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"id": saved.ID, "status": "captured"}, nil
}

func (h *Handler) memMergeProjects(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	from := str(args, "from")
	to := str(args, "to")
	if from == "" || to == "" {
		return nil, fmt.Errorf("from and to are required")
	}
	count, err := h.store.MergeProjects(ctx, from, to)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"status":  "merged",
		"from":    from,
		"to":      to,
		"count":   count,
	}, nil
}

func (h *Handler) memSemanticSearch(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	query := str(args, "query")
	project := str(args, "project")
	limit := int(num(args, "limit"))

	// Generate embedding for the query text.
	emb, err := h.embedder.Embed(ctx, query)
	if err != nil {
		// Embedding not configured or temporarily unavailable — fall back to FTS.
		return h.memSearch(ctx, args)
	}

	results, err := h.store.SemanticSearch(ctx, emb, project, limit)
	if err != nil {
		return nil, err
	}

	out := make([]map[string]interface{}, 0, len(results))
	for _, r := range results {
		out = append(out, map[string]interface{}{
			"id":         r.ID,
			"title":      r.Title,
			"type":       r.Type,
			"project":    r.Project,
			"snippet":    r.Snippet,
			"similarity": r.Rank, // cosine similarity 0-1
			"created_at": r.CreatedAt,
		})
	}
	return map[string]interface{}{
		"results": out,
		"count":   len(out),
		"mode":    "semantic",
	}, nil
}

func (h *Handler) memSaveAttachment(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	observationID := int64(num(args, "observation_id"))
	if observationID <= 0 {
		return nil, fmt.Errorf("observation_id is required and must be > 0")
	}
	name := str(args, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	mime := strDefault(args, "mime", "application/octet-stream")
	dataB64 := str(args, "data_b64")
	if dataB64 == "" {
		return nil, fmt.Errorf("data_b64 is required")
	}
	data, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 data")
	}
	att, err := h.store.SaveAttachment(ctx, observationID, name, mime, data)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"status": "saved", "id": att.ID, "observation_id": att.ObservationID, "sha256": att.SHA256}, nil
}

func (h *Handler) memListAttachments(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	observationID := int64(num(args, "observation_id"))
	if observationID <= 0 {
		return nil, fmt.Errorf("observation_id is required and must be > 0")
	}
	items, err := h.store.ListAttachments(ctx, observationID)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"attachments": items, "count": len(items)}, nil
}

func (h *Handler) memSaveRelation(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	fromID := int64(num(args, "from_id"))
	toID := int64(num(args, "to_id"))
	relType := strDefault(args, "type", "related")
	rel, err := h.store.SaveRelation(ctx, fromID, toID, relType)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"status": "saved", "id": rel.ID, "from_id": rel.FromID, "to_id": rel.ToID, "type": rel.Type}, nil
}

func (h *Handler) memListRelations(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	id := int64(num(args, "observation_id"))
	if id <= 0 {
		return nil, fmt.Errorf("observation_id is required and must be > 0")
	}
	items, err := h.store.ListRelations(ctx, id)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"relations": items, "count": len(items)}, nil
}

// ─────────────────────────────────────────────────────────────────
// Manifest helpers
// ─────────────────────────────────────────────────────────────────

func tool(name, desc string, extraProps ...interface{}) map[string]interface{} {
	props := map[string]interface{}{}
	var required []string
	for _, p := range extraProps {
		switch v := p.(type) {
		case map[string]interface{}:
			for k, val := range v {
				props[k] = val
			}
		case []string:
			required = v
		}
	}
	schema := map[string]interface{}{
		"type":       "object",
		"properties": props,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return map[string]interface{}{
		"name":        name,
		"description": desc,
		"inputSchema": schema,
	}
}

func prop(name, typ, desc string) map[string]interface{} {
	schema := map[string]interface{}{"type": typ, "description": desc}
	if typ == "array" {
		schema["items"] = map[string]interface{}{"type": "string"}
	}
	return map[string]interface{}{name: schema}
}

// ─────────────────────────────────────────────────────────────────
// Arg helpers
// ─────────────────────────────────────────────────────────────────

func str(args map[string]interface{}, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
		b, _ := json.Marshal(v)
		return string(b)
	}
	return ""
}

func strDefault(args map[string]interface{}, key, def string) string {
	s := str(args, key)
	if s == "" {
		return def
	}
	return s
}

func num(args map[string]interface{}, key string) float64 {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case int:
			return float64(n)
		case int64:
			return float64(n)
		case json.Number:
			f, _ := n.Float64()
			return f
		}
	}
	return 0
}

func boolArg(args map[string]interface{}, key string) bool {
	if v, ok := args[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func stringSlice(args map[string]interface{}, key string) []string {
	v, ok := args[key]
	if !ok || v == nil {
		return nil
	}
	switch raw := v.(type) {
	case []string:
		return raw
	case []interface{}:
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
