package mcp_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	mcppkg "github.com/mdesantis1984/ia-recuerdo/internal/mcp"
	"github.com/mdesantis1984/ia-recuerdo/internal/store"
)

func newTestHandler(t *testing.T) *mcppkg.Handler {
	t.Helper()
	dsn := os.Getenv("IA_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("IA_TEST_PG_DSN not set")
	}
	s, err := store.New(context.Background(), store.Config{Driver: "postgres", DSN: dsn})
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return mcppkg.New(s, nil, nil) // nil = no-op cache and embedding in tests
}

// ─────────────────────────────────────────────────────────────────
// Tools manifest
// ─────────────────────────────────────────────────────────────────

func TestToolList_Has20Tools(t *testing.T) {
	tools := mcppkg.ToolList()
	if len(tools) != 20 {
		t.Fatalf("expected 20 tools, got %d", len(tools))
	}
}

func TestToolList_HasTagFilters(t *testing.T) {
	for _, tool := range mcppkg.ToolList() {
		if tool["name"] == "mem_search" || tool["name"] == "mem_context" {
			schema := tool["inputSchema"].(map[string]interface{})
			props := schema["properties"].(map[string]interface{})
			if _, ok := props["tags"]; !ok {
				t.Fatalf("tool %v missing tags property", tool["name"])
			}
		}
	}
}

func TestToolList_HasSemanticSearch(t *testing.T) {
	for _, tool := range mcppkg.ToolList() {
		if tool["name"] == "mem_semantic_search" {
			return
		}
	}
	t.Fatal("mem_semantic_search not found in tool list")
}

func TestToolList_AllHaveRequiredFields(t *testing.T) {
	for _, tool := range mcppkg.ToolList() {
		if tool["name"] == "" || tool["name"] == nil {
			t.Errorf("tool missing name: %v", tool)
		}
		if tool["description"] == "" || tool["description"] == nil {
			t.Errorf("tool %v missing description", tool["name"])
		}
		if tool["inputSchema"] == nil {
			t.Errorf("tool %v missing inputSchema", tool["name"])
		}
	}
}

// ─────────────────────────────────────────────────────────────────
// mem_save / mem_search / mem_get_observation
// ─────────────────────────────────────────────────────────────────

func TestMemSave(t *testing.T) {
	h := newTestHandler(t)
	result, err := h.Call(context.Background(), "mem_save", map[string]interface{}{
		"title":   "Test memory",
		"content": "This is test content for ia-recuerdo",
		"type":    "discovery",
		"project": "test",
	})
	if err != nil {
		t.Fatalf("mem_save: %v", err)
	}
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	if m["status"] != "saved" {
		t.Fatalf("expected status=saved, got %v", m["status"])
	}
	if m["id"] == nil || m["id"].(int64) == 0 {
		t.Fatal("expected non-zero ID in response")
	}
}

func TestMemSave_WithTopicKey(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()

	args := map[string]interface{}{
		"title":     "DB decision",
		"content":   "PostgreSQL chosen",
		"type":      "decision",
		"project":   "infra",
		"topic_key": "architecture/db",
	}
	r1, _ := h.Call(ctx, "mem_save", args)
	r2, _ := h.Call(ctx, "mem_save", args) // upsert

	id1 := r1.(map[string]interface{})["id"].(int64)
	id2 := r2.(map[string]interface{})["id"].(int64)
	if id1 != id2 {
		t.Fatalf("upsert: expected same ID, got %d vs %d", id1, id2)
	}
}

func TestMemSearch(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()

	h.Call(ctx, "mem_save", map[string]interface{}{
		"title": "Kubernetes deploy", "content": "Deployed to k8s cluster",
		"type": "config", "project": "infra",
	})

	result, err := h.Call(ctx, "mem_search", map[string]interface{}{
		"query":   "Kubernetes",
		"project": "infra",
	})
	if err != nil {
		t.Fatalf("mem_search: %v", err)
	}
	m := result.(map[string]interface{})
	count := m["count"].(int)
	if count == 0 {
		t.Fatal("expected at least 1 result for 'Kubernetes'")
	}
}

func TestMemSearch_WithTags(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()

	h.Call(ctx, "mem_save", map[string]interface{}{
		"title": "Tagged obs", "content": "content", "type": "discovery", "project": "infra", "tags": []interface{}{"pgvector", "db"},
	})

	result, err := h.Call(ctx, "mem_search", map[string]interface{}{
		"query": "Tagged",
		"project": "infra",
		"tags": []interface{}{"pgvector"},
	})
	if err != nil {
		t.Fatalf("mem_search tags: %v", err)
	}
	m := result.(map[string]interface{})
	if m["count"].(int) == 0 {
		t.Fatal("expected tagged result")
	}
}

func TestMemGetObservation(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()

	saved, _ := h.Call(ctx, "mem_save", map[string]interface{}{
		"title": "Fetchable obs", "content": "full content here",
		"type": "learning", "project": "p",
	})
	id := saved.(map[string]interface{})["id"].(int64)

	result, err := h.Call(ctx, "mem_get_observation", map[string]interface{}{"id": float64(id)})
	if err != nil {
		t.Fatalf("mem_get_observation: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil observation")
	}
}

func TestMemGetObservation_Unknown(t *testing.T) {
	h := newTestHandler(t)
	_, err := h.Call(context.Background(), "mem_get_observation", map[string]interface{}{"id": float64(99999)})
	if err == nil {
		t.Fatal("expected error for unknown ID")
	}
}

// ─────────────────────────────────────────────────────────────────
// mem_update / mem_delete
// ─────────────────────────────────────────────────────────────────

func TestMemUpdate(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()

	saved, _ := h.Call(ctx, "mem_save", map[string]interface{}{
		"title": "Original", "content": "old", "type": "discovery", "project": "p",
	})
	id := saved.(map[string]interface{})["id"].(int64)

	result, err := h.Call(ctx, "mem_update", map[string]interface{}{
		"id":      float64(id),
		"title":   "Updated title",
		"content": "new content",
	})
	if err != nil {
		t.Fatalf("mem_update: %v", err)
	}
	m := result.(map[string]interface{})
	if m["status"] != "updated" {
		t.Fatalf("expected status=updated, got %v", m["status"])
	}
}

func TestMemDelete_Soft(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()

	saved, _ := h.Call(ctx, "mem_save", map[string]interface{}{
		"title": "Temp", "content": "bye", "type": "discovery", "project": "p",
	})
	id := saved.(map[string]interface{})["id"].(int64)

	result, err := h.Call(ctx, "mem_delete", map[string]interface{}{"id": float64(id), "hard": false})
	if err != nil {
		t.Fatalf("mem_delete: %v", err)
	}
	m := result.(map[string]interface{})
	if m["status"] != "deleted" {
		t.Fatalf("expected status=deleted, got %v", m["status"])
	}
}

// ─────────────────────────────────────────────────────────────────
// mem_suggest_topic_key
// ─────────────────────────────────────────────────────────────────

func TestMemSuggestTopicKey(t *testing.T) {
	h := newTestHandler(t)
	result, err := h.Call(context.Background(), "mem_suggest_topic_key", map[string]interface{}{
		"type":  "architecture",
		"title": "Database selection for production",
	})
	if err != nil {
		t.Fatalf("mem_suggest_topic_key: %v", err)
	}
	m := result.(map[string]interface{})
	key, ok := m["topic_key"].(string)
	if !ok || key == "" {
		t.Fatalf("expected non-empty topic_key, got %v", m)
	}
	if !strings.Contains(key, "/") {
		t.Fatalf("expected topic_key with family/, got %q", key)
	}
}

// ─────────────────────────────────────────────────────────────────
// Sessions
// ─────────────────────────────────────────────────────────────────

func TestMemSessionStartEnd(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()

	startResult, err := h.Call(ctx, "mem_session_start", map[string]interface{}{
		"project": "myproject",
		"agent":   "vscode",
		"goal":    "Build IA_Recuerdo",
	})
	if err != nil {
		t.Fatalf("mem_session_start: %v", err)
	}
	m := startResult.(map[string]interface{})
	if m["status"] != "session_started" {
		t.Fatalf("expected session_started, got %v", m["status"])
	}
	sessionID := m["session_id"].(string)

	_, err = h.Call(ctx, "mem_session_end", map[string]interface{}{
		"session_id": sessionID,
		"summary":    "Completed implementation",
	})
	if err != nil {
		t.Fatalf("mem_session_end: %v", err)
	}
}

func TestMemSessionSummary(t *testing.T) {
	h := newTestHandler(t)
	result, err := h.Call(context.Background(), "mem_session_summary", map[string]interface{}{
		"goal":         "Implement feature X",
		"accomplished": "Done",
		"discoveries":  "FTS5 works well",
		"files":        "store.go, store_test.go",
		"project":      "test",
	})
	if err != nil {
		t.Fatalf("mem_session_summary: %v", err)
	}
	m := result.(map[string]interface{})
	if m["status"] != "summary_saved" {
		t.Fatalf("expected summary_saved, got %v", m["status"])
	}
}

// ─────────────────────────────────────────────────────────────────
// mem_context / mem_timeline
// ─────────────────────────────────────────────────────────────────

func TestMemContext(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		h.Call(ctx, "mem_save", map[string]interface{}{
			"title": "Context obs", "content": "content", "type": "discovery", "project": "ctx-proj",
		})
	}

	result, err := h.Call(ctx, "mem_context", map[string]interface{}{"project": "ctx-proj", "limit": float64(5)})
	if err != nil {
		t.Fatalf("mem_context: %v", err)
	}
	m := result.(map[string]interface{})
	if m["count"].(int) < 1 {
		t.Fatal("expected at least 1 observation in context")
	}
}

func TestMemContext_WithTags(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()

	h.Call(ctx, "mem_save", map[string]interface{}{
		"title": "Ctx tagged", "content": "content", "type": "discovery", "project": "ctx-proj", "tags": []interface{}{"blue"},
	})

	result, err := h.Call(ctx, "mem_context", map[string]interface{}{"project": "ctx-proj", "tags": []interface{}{"blue"}, "limit": float64(5)})
	if err != nil {
		t.Fatalf("mem_context tags: %v", err)
	}
	m := result.(map[string]interface{})
	if m["count"].(int) == 0 {
		t.Fatal("expected tagged context result")
	}
}

func TestMemTimeline(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()

	var mid float64
	for i := 0; i < 5; i++ {
		r, _ := h.Call(ctx, "mem_save", map[string]interface{}{
			"title": "Timeline obs", "content": "c", "type": "discovery", "project": "tl",
		})
		if i == 2 {
			mid = float64(r.(map[string]interface{})["id"].(int64))
		}
	}

	result, err := h.Call(ctx, "mem_timeline", map[string]interface{}{
		"observation_id": mid,
		"window":         float64(2),
	})
	if err != nil {
		t.Fatalf("mem_timeline: %v", err)
	}
	m := result.(map[string]interface{})
	if m["count"].(int) == 0 {
		t.Fatal("expected non-empty timeline")
	}
}

// ─────────────────────────────────────────────────────────────────
// mem_stats / mem_save_prompt / mem_capture_passive
// ─────────────────────────────────────────────────────────────────

func TestMemStats(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()

	h.Call(ctx, "mem_save", map[string]interface{}{
		"title": "A", "content": "B", "type": "discovery", "project": "p",
	})

	result, err := h.Call(ctx, "mem_stats", map[string]interface{}{})
	if err != nil {
		t.Fatalf("mem_stats: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil stats")
	}
}

func TestMemSavePrompt(t *testing.T) {
	h := newTestHandler(t)
	result, err := h.Call(context.Background(), "mem_save_prompt", map[string]interface{}{
		"content": "You are an expert Go developer.",
		"project": "default",
	})
	if err != nil {
		t.Fatalf("mem_save_prompt: %v", err)
	}
	m := result.(map[string]interface{})
	if m["status"] != "prompt_saved" {
		t.Fatalf("expected prompt_saved, got %v", m["status"])
	}
}

func TestMemCapturePassive_Short(t *testing.T) {
	h := newTestHandler(t)
	result, err := h.Call(context.Background(), "mem_capture_passive", map[string]interface{}{
		"text": "too short",
	})
	if err != nil {
		t.Fatalf("mem_capture_passive: %v", err)
	}
	m := result.(map[string]interface{})
	if m["status"] != "skipped" {
		t.Fatalf("expected skipped for short text, got %v", m["status"])
	}
}

func TestMemCapturePassive_Long(t *testing.T) {
	h := newTestHandler(t)
	longText := strings.Repeat("This is an important discovery about the system. ", 10)
	result, err := h.Call(context.Background(), "mem_capture_passive", map[string]interface{}{
		"text":    longText,
		"project": "p",
	})
	if err != nil {
		t.Fatalf("mem_capture_passive: %v", err)
	}
	m := result.(map[string]interface{})
	if m["status"] != "captured" {
		t.Fatalf("expected captured, got %v", m["status"])
	}
}

func TestMemMergeProjects(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()

	h.Call(ctx, "mem_save", map[string]interface{}{
		"title": "Project A", "content": "content", "type": "discovery", "project": "from-proj",
	})

	result, err := h.Call(ctx, "mem_merge_projects", map[string]interface{}{"from": "from-proj", "to": "to-proj"})
	if err != nil {
		t.Fatalf("mem_merge_projects: %v", err)
	}
	m := result.(map[string]interface{})
	if m["status"] != "merged" {
		t.Fatalf("expected merged, got %v", m["status"])
	}
}

func TestMemSaveAttachmentAndList(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()

	saved, _ := h.Call(ctx, "mem_save", map[string]interface{}{
		"title": "Attachment parent", "content": "content", "type": "discovery", "project": "att",
	})
	id := saved.(map[string]interface{})["id"].(int64)

	result, err := h.Call(ctx, "mem_save_attachment", map[string]interface{}{
		"observation_id": float64(id),
		"name": "note.txt",
		"mime": "text/plain",
		"data_b64": base64.StdEncoding.EncodeToString([]byte("hello")),
	})
	if err != nil {
		t.Fatalf("mem_save_attachment: %v", err)
	}
	if result.(map[string]interface{})["status"] != "saved" {
		t.Fatalf("expected saved, got %v", result)
	}

	list, err := h.Call(ctx, "mem_list_attachments", map[string]interface{}{"observation_id": float64(id)})
	if err != nil {
		t.Fatalf("mem_list_attachments: %v", err)
	}
	m := list.(map[string]interface{})
	if m["count"].(int) == 0 {
		t.Fatal("expected at least one attachment")
	}
}

func TestMemSaveRelationAndList(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()

	a, _ := h.Call(ctx, "mem_save", map[string]interface{}{
		"title": "Relation A", "content": "content", "type": "discovery", "project": "rel",
	})
	b, _ := h.Call(ctx, "mem_save", map[string]interface{}{
		"title": "Relation B", "content": "content", "type": "discovery", "project": "rel",
	})
	idA := a.(map[string]interface{})["id"].(int64)
	idB := b.(map[string]interface{})["id"].(int64)

	result, err := h.Call(ctx, "mem_save_relation", map[string]interface{}{
		"from_id": float64(idA),
		"to_id": float64(idB),
		"type": "related",
	})
	if err != nil {
		t.Fatalf("mem_save_relation: %v", err)
	}
	if result.(map[string]interface{})["status"] != "saved" {
		t.Fatalf("expected saved, got %v", result)
	}

	list, err := h.Call(ctx, "mem_list_relations", map[string]interface{}{"observation_id": float64(idA)})
	if err != nil {
		t.Fatalf("mem_list_relations: %v", err)
	}
	m := list.(map[string]interface{})
	if m["count"].(int) == 0 {
		t.Fatal("expected at least one relation")
	}
}

// ─────────────────────────────────────────────────────────────────
// Unknown tool
// ─────────────────────────────────────────────────────────────────

func TestCall_UnknownTool(t *testing.T) {
	h := newTestHandler(t)
	_, err := h.Call(context.Background(), "mem_nonexistent", map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

// ─────────────────────────────────────────────────────────────────
// mem_semantic_search (falls back to text search when no embedder is configured)
// ─────────────────────────────────────────────────────────────────

func TestMemSemanticSearch_FallsBackToFTS(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()

	h.Call(ctx, "mem_save", map[string]interface{}{
		"title": "Valkey semantic test", "content": "Testing semantic fallback with FTS", "type": "discovery", "project": "sem",
	})

	// No embedder configured (nil) → transparently falls back to FTS search.
	result, err := h.Call(ctx, "mem_semantic_search", map[string]interface{}{
		"query": "Valkey", "project": "sem",
	})
	if err != nil {
		t.Fatalf("mem_semantic_search: %v", err)
	}
	m := result.(map[string]interface{})
	count, ok := m["count"].(int)
	if !ok || count < 1 {
		t.Fatalf("expected at least 1 result, got %v", m)
	}
}

// ─────────────────────────────────────────────────────────────────
// HTTP transport
// ─────────────────────────────────────────────────────────────────

func TestHTTPRPC_Initialize(t *testing.T) {
	h := newTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterHTTPRoutes(mux)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
	req := httptest.NewRequest("POST", "/mcp/rpc", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["result"] == nil {
		t.Fatal("expected result in response")
	}
}

func TestHTTPRPC_ToolsList(t *testing.T) {
	h := newTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterHTTPRoutes(mux)

	body := `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`
	req := httptest.NewRequest("POST", "/mcp/rpc", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHTTPRPC_ToolCall(t *testing.T) {
	h := newTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterHTTPRoutes(mux)

	body := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"mem_stats","arguments":{}}}`
	req := httptest.NewRequest("POST", "/mcp/rpc", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHTTPRPC_BadJSON(t *testing.T) {
	h := newTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterHTTPRoutes(mux)

	req := httptest.NewRequest("POST", "/mcp/rpc", strings.NewReader("{bad json"))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHTTPToolList(t *testing.T) {
	h := newTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterHTTPRoutes(mux)

	req := httptest.NewRequest("GET", "/mcp/tools", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	tools, ok := resp["tools"].([]interface{})
		if !ok || len(tools) != 20 {
			t.Fatalf("expected 20 tools in GET /mcp/tools, got %v", resp["tools"])
		}
}

func TestHTTPBatch(t *testing.T) {
	h := newTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterHTTPRoutes(mux)

	body := `[
		{"jsonrpc":"2.0","id":1,"method":"tools/list"},
		{"jsonrpc":"2.0","id":2,"method":"mcp.tools.list"}
	]`
	req := httptest.NewRequest("POST", "/mcp/rpc/batch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resps []map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resps)
	if len(resps) != 2 {
		t.Fatalf("expected 2 batch responses, got %d", len(resps))
	}
}

// ─────────────────────────────────────────────────────────────────
// Streamable HTTP transport  (MCP 2025-03-26 — VS Code, Visual Studio, OpenClaw)
// ─────────────────────────────────────────────────────────────────

func newStreamableMux(t *testing.T) http.Handler {
	t.Helper()
	h := newTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterHTTPRoutes(mux)
	return mux
}

// VS Code sends Accept: application/json, text/event-stream and expects SSE response.
func TestStreamable_InitializeReturnsSSE(t *testing.T) {
	mux := newStreamableMux(t)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("expected text/event-stream, got %q", ct)
	}
	resp := rr.Body.String()
	if !strings.HasPrefix(resp, "data: ") {
		t.Fatalf("expected SSE data: line, got %q", resp)
	}
	if !strings.Contains(resp, "\n\n") {
		t.Fatalf("expected SSE double-newline terminator, got %q", resp)
	}
}

// When Accept is only application/json, respond with plain JSON.
func TestStreamable_InitializeReturnsJSON(t *testing.T) {
	mux := newStreamableMux(t)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Fatalf("expected application/json, got %q", ct)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("expected valid JSON response: %v", err)
	}
	if resp["result"] == nil {
		t.Fatal("expected result field in response")
	}
}

// notifications/initialized must return 202 Accepted with no body.
func TestStreamable_NotificationReturns202(t *testing.T) {
	mux := newStreamableMux(t)

	body := `{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for notification, got %d", rr.Code)
	}
}

// tools/list via Streamable HTTP returns 20 tools.
func TestStreamable_ToolsListSSE(t *testing.T) {
	mux := newStreamableMux(t)

	body := `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	raw := rr.Body.String()
	// SSE line: "data: {...}\n\n"
	if !strings.HasPrefix(raw, "data: ") {
		t.Fatalf("expected SSE format, got %q", raw)
	}
	jsonPart := strings.TrimPrefix(strings.TrimSpace(raw), "data: ")
	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(jsonPart), &resp); err != nil {
		t.Fatalf("invalid JSON inside SSE: %v", err)
	}
	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected result object, got %v", resp)
	}
	tools, ok := result["tools"].([]interface{})
	if !ok || len(tools) != 20 {
		t.Fatalf("expected 20 tools, got %v", result["tools"])
	}
}

// DELETE /mcp must return 200 (session termination is a no-op for stateless server).
func TestStreamable_DeleteReturns200(t *testing.T) {
	mux := newStreamableMux(t)

	req := httptest.NewRequest("DELETE", "/mcp", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for DELETE, got %d", rr.Code)
	}
}

// OPTIONS /mcp must return CORS headers (preflight).
func TestStreamable_OptionsPreflight(t *testing.T) {
	mux := newStreamableMux(t)

	req := httptest.NewRequest("OPTIONS", "/mcp", nil)
	req.Header.Set("Origin", "vscode-webview://abc")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for OPTIONS, got %d", rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Fatal("expected Access-Control-Allow-Origin header on OPTIONS")
	}
}

// Mcp-Session-Id header must be echoed back.
func TestStreamable_SessionIdEchoed(t *testing.T) {
	mux := newStreamableMux(t)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Mcp-Session-Id", "test-session-42")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if got := rr.Header().Get("Mcp-Session-Id"); got != "test-session-42" {
		t.Fatalf("expected Mcp-Session-Id=test-session-42, got %q", got)
	}
}

// ping method must return empty result.
func TestStreamable_Ping(t *testing.T) {
	mux := newStreamableMux(t)

	body := `{"jsonrpc":"2.0","id":99,"method":"ping"}`
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for ping, got %d", rr.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["result"] == nil {
		t.Fatal("expected result for ping")
	}
}
