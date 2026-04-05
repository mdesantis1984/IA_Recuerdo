// Package mcp — STDIO and HTTP MCP transports.
// STDIO: for local agent integration (compatible with MCP spec)
// HTTP:  THE KEY DIFFERENTIATOR — remote access without installing any binary
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

// ─────────────────────────────────────────────────────────────────
// JSON-RPC 2.0 wire types
// ─────────────────────────────────────────────────────────────────

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
}

type rpcError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func errResp(id interface{}, code int, msg string) *rpcResponse {
	return &rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}}
}

// ─────────────────────────────────────────────────────────────────
// Core dispatch
// ─────────────────────────────────────────────────────────────────

// dispatch handles a single JSON-RPC request and returns a response.
// Returns nil for JSON-RPC notifications (no ID) — callers must not send a response.
func (h *Handler) dispatch(ctx context.Context, req *rpcRequest) *rpcResponse {
	// JSON-RPC notifications have no "id". Accept them silently.
	isNotification := req.ID == nil

	switch req.Method {
	case "initialize":
		if isNotification {
			return nil
		}
		return h.handleInitialize(req)

	case "notifications/initialized", "notifications/cancelled", "notifications/progress":
		return nil // always silent

	case "ping":
		if isNotification {
			return nil
		}
		return &rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]interface{}{}}

	case "mcp.tools.list", "tools/list":
		if isNotification {
			return nil
		}
		return &rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]interface{}{
			"tools": ToolList(),
		}}

	case "mcp.tools.call", "tools/call":
		if isNotification {
			return nil
		}
		var params struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return errResp(req.ID, -32602, "invalid params")
		}
		result, err := h.Call(ctx, params.Name, params.Arguments)
		if err != nil {
			return errResp(req.ID, -32603, err.Error())
		}
		return &rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": mustJSON(result)},
			},
		}}

	default:
		if isNotification {
			return nil // unknown notifications are silently ignored per spec
		}
		return errResp(req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method))
	}
}

func (h *Handler) handleInitialize(req *rpcRequest) *rpcResponse {
	return &rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"serverInfo": map[string]interface{}{
			"name":    "ia-recuerdo",
			"version": "1.0.0",
		},
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{"listChanged": false},
		},
	}}
}

// ─────────────────────────────────────────────────────────────────
// STDIO transport (local agent, newline-delimited JSON-RPC)
// ─────────────────────────────────────────────────────────────────

// ServeStdio runs the MCP server on stdin/stdout.
func (h *Handler) ServeStdio(ctx context.Context) error {
	log.Println("[mcp/stdio] Ready")
	reader := bufio.NewReader(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var req rpcRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = encoder.Encode(errResp(nil, -32700, "parse error"))
			continue
		}

		resp := h.dispatch(ctx, &req)
		if resp != nil {
			_ = encoder.Encode(resp)
		}
	}
}

// ─────────────────────────────────────────────────────────────────
// HTTP MCP transport (THE KEY FEATURE — remote without local binary)
// ─────────────────────────────────────────────────────────────────

// RegisterHTTPRoutes mounts the MCP HTTP endpoints on the given mux.
//
// Streamable HTTP (MCP 2025-03-26) — VS Code, Visual Studio, OpenClaw:
//
//	POST /mcp          → Streamable HTTP (single + SSE response)
//	DELETE /mcp        → terminate session (no-op, returns 200)
//
// Legacy JSON-RPC (backward compat):
//
//	POST /mcp/rpc      → JSON-RPC 2.0 single
//	POST /mcp/rpc/batch → JSON-RPC 2.0 batch
//	GET  /mcp/tools    → list available tools (convenience)
func (h *Handler) RegisterHTTPRoutes(mux *http.ServeMux) {
	// Streamable HTTP — handle all methods on /mcp (Go 1.22 wildcard w/o method prefix)
	mux.HandleFunc("/mcp", h.httpStreamableDispatch)

	// Legacy endpoints (backward compat — keep existing clients working)
	mux.HandleFunc("POST /mcp/rpc", h.httpRPC)
	mux.HandleFunc("POST /mcp/rpc/batch", h.httpBatch)
	mux.HandleFunc("GET /mcp/tools", h.httpToolList)
}

// httpStreamableDispatch routes GET/POST/DELETE/OPTIONS to the correct handler.
func (h *Handler) httpStreamableDispatch(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w, r)
	switch r.Method {
	case http.MethodPost:
		h.httpStreamablePost(w, r)
	case http.MethodDelete:
		// Session termination — stateless server, nothing to clean up
		w.WriteHeader(http.StatusOK)
	case http.MethodOptions:
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// httpStreamablePost handles a single JSON-RPC POST from a Streamable HTTP client.
// It selects the response format based on the Accept header:
//   - Accept: application/json         → plain JSON (VS Code, cURL)
//   - Accept: text/event-stream        → SSE-wrapped JSON (some clients)
//   - Accept: application/json, text/event-stream → SSE (MCP SDK default)
func (h *Handler) httpStreamablePost(w http.ResponseWriter, r *http.Request) {
	var req rpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		setCORSHeaders(w, r)
		writeJSON(w, http.StatusBadRequest, errResp(nil, -32700, "parse error"))
		return
	}

	resp := h.dispatch(r.Context(), &req)

	// Notification — no body, 202 Accepted
	if resp == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// Echo back session ID if the client sent one (stateless — we don't enforce it)
	if sid := r.Header.Get("Mcp-Session-Id"); sid != "" {
		w.Header().Set("Mcp-Session-Id", sid)
	}

	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "text/event-stream") {
		// SSE response format
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		b, _ := json.Marshal(resp)
		fmt.Fprintf(w, "data: %s\n\n", b)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		return
	}

	// Default: plain JSON
	code := http.StatusOK
	if resp.Error != nil {
		code = http.StatusUnprocessableEntity
	}
	writeJSON(w, code, resp)
}

func (h *Handler) httpRPC(w http.ResponseWriter, r *http.Request) {
	var req rpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errResp(nil, -32700, "parse error"))
		return
	}
	resp := h.dispatch(r.Context(), &req)
	code := http.StatusOK
	if resp.Error != nil {
		code = http.StatusUnprocessableEntity
	}
	writeJSON(w, code, resp)
}

func (h *Handler) httpBatch(w http.ResponseWriter, r *http.Request) {
	var reqs []rpcRequest
	if err := json.NewDecoder(r.Body).Decode(&reqs); err != nil {
		writeJSON(w, http.StatusBadRequest, errResp(nil, -32700, "parse error"))
		return
	}
	resps := make([]*rpcResponse, 0, len(reqs))
	for i := range reqs {
		resps = append(resps, h.dispatch(r.Context(), &reqs[i]))
	}
	writeJSON(w, http.StatusOK, resps)
}

func (h *Handler) httpToolList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"tools": ToolList()})
}

// ─────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────

// setCORSHeaders sets permissive CORS headers required for VS Code / browser clients.
func setCORSHeaders(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = "*"
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Mcp-Session-Id, Authorization")
	w.Header().Set("Access-Control-Expose-Headers", "Mcp-Session-Id")
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func mustJSON(v interface{}) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error":"%s"}`, err)
	}
	return string(b)
}
