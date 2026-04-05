// Package embedding provides a pluggable interface for generating text embeddings.
// Works with Ollama (local, free) and any OpenAI-compatible API (OpenAI, Together, etc.).
//
// Formats supported:
//
//	openai  — POST {"model":..., "input":...}  → {"data":[{"embedding":[...]}]}
//	ollama  — POST {"model":..., "prompt":...} → {"embedding":[...]}
//
// Ollama also exposes an OpenAI-compatible endpoint at /v1/embeddings, so both formats work.
package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// ErrNotConfigured is returned when no embedding provider is set.
var ErrNotConfigured = errors.New("embedding provider not configured")

// Provider generates a dense vector from a text string.
type Provider interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	// Dims returns the vector dimension. 0 means provider is disabled.
	Dims() int
}

// ─────────────────────────────────────────────────────────────────
// Disabled (no-op)
// ─────────────────────────────────────────────────────────────────

// Disabled is a no-op provider used when no embedding is configured.
type Disabled struct{}

func (d *Disabled) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, ErrNotConfigured
}
func (d *Disabled) Dims() int { return 0 }

// ─────────────────────────────────────────────────────────────────
// HTTP Provider (OpenAI + Ollama native formats)
// ─────────────────────────────────────────────────────────────────

// HTTPProvider calls any OpenAI-compatible or Ollama-native embedding API.
type HTTPProvider struct {
	url    string
	model  string
	token  string
	format string // "openai" | "ollama"
	dims   int
	client *http.Client
}

// New creates a new HTTP embedding provider.
//
//	url    — full endpoint URL:
//	         Ollama OpenAI-compat: http://localhost:11434/v1/embeddings
//	         OpenAI:               https://api.openai.com/v1/embeddings
//	         Ollama native:        http://localhost:11434/api/embeddings
//	model  — e.g. "nomic-embed-text" (Ollama) or "text-embedding-3-small" (OpenAI)
//	token  — Bearer token (empty for local Ollama)
//	format — "openai" or "ollama" (default: "openai")
//	dims   — embedding dimension (768 for nomic-embed-text, 1536 for text-embedding-3-small)
func New(url, model, token, format string, dims int) *HTTPProvider {
	if format == "" {
		format = "openai"
	}
	return &HTTPProvider{
		url:    url,
		model:  model,
		token:  token,
		format: format,
		dims:   dims,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (h *HTTPProvider) Dims() int { return h.dims }

// Embed generates a vector for the given text.
func (h *HTTPProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	switch h.format {
	case "ollama":
		return h.embedOllama(ctx, text)
	default: // "openai" + any OpenAI-compatible
		return h.embedOpenAI(ctx, text)
	}
}

// embedOpenAI uses the OpenAI /v1/embeddings schema (also supported by Ollama).
func (h *HTTPProvider) embedOpenAI(ctx context.Context, text string) ([]float32, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"model": h.model,
		"input": text,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if h.token != "" {
		req.Header.Set("Authorization", "Bearer "+h.token)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding API %s: HTTP %d", h.url, resp.StatusCode)
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode embedding response: %w", err)
	}
	if len(result.Data) == 0 || len(result.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("empty embedding response from %s", h.url)
	}
	return result.Data[0].Embedding, nil
}

// embedOllama uses the Ollama-native /api/embeddings schema.
func (h *HTTPProvider) embedOllama(ctx context.Context, text string) ([]float32, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"model":  h.model,
		"prompt": text,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if h.token != "" {
		req.Header.Set("Authorization", "Bearer "+h.token)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding API %s: HTTP %d", h.url, resp.StatusCode)
	}

	var result struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode embedding response: %w", err)
	}
	if len(result.Embedding) == 0 {
		return nil, fmt.Errorf("empty embedding response from %s", h.url)
	}
	return result.Embedding, nil
}
