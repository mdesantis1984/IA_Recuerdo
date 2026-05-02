// Command ia-recuerdo — servidor MCP de memoria persistente para agentes IA.
//
// Usage examples:
//
//	ia-recuerdo -transport stdio                       # local MCP (stdio)
//	ia-recuerdo -transport http -addr :7438            # remote HTTP MCP
//	ia-recuerdo -transport both -addr :7438            # both simultaneously
//	ia-recuerdo -db-driver postgres -db-dsn "..."      # PostgreSQL production
//	ia-recuerdo -valkey localhost:6379                 # enable Valkey cache
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/mdesantis1984/ia-recuerdo/internal/cache"
	"github.com/mdesantis1984/ia-recuerdo/internal/embedding"
	"github.com/mdesantis1984/ia-recuerdo/internal/mcp"
	"github.com/mdesantis1984/ia-recuerdo/internal/server"
	"github.com/mdesantis1984/ia-recuerdo/internal/store"
	"github.com/mdesantis1984/ia-recuerdo/pkg/types"
)

var version = "dev"

func main() {
	transport  := flag.String("transport",    envOr("IA_TRANSPORT", "http"),          "Transport: stdio|http|both")
	addr       := flag.String("addr",         envOr("IA_ADDR", ":7438"),              "HTTP listen address")
	dbDriver   := flag.String("db-driver",    envOr("IA_DB_DRIVER", "postgres"),      "DB driver: postgres")
	dbDSN      := flag.String("db-dsn",       envOr("IA_DB_DSN", ""),                 "PostgreSQL DSN")
	embedURL   := flag.String("embed-url",    envOr("IA_EMBED_URL", ""),              "Embedding API URL (e.g. http://ollama:11434/v1/embeddings)")
	embedModel := flag.String("embed-model",  envOr("IA_EMBED_MODEL", "nomic-embed-text"), "Embedding model name")
	embedToken := flag.String("embed-token",  envOr("IA_EMBED_TOKEN", ""),            "Embedding API Bearer token (optional for local Ollama)")
	embedFmt   := flag.String("embed-format", envOr("IA_EMBED_FORMAT", "openai"),    "Embedding API format: openai|ollama")
	embedDims  := flag.Int("embed-dims",      envOrInt("IA_EMBED_DIMS", 768),         "Embedding vector dimensions (768=nomic-embed-text, 1536=OpenAI)")
	valkeyAddr := flag.String("valkey",        envOr("IA_VALKEY", ""),                "Valkey address (host:port). Empty = disabled")
	upsertEnabled  := flag.Bool("upsert-enabled",  envOrBool("IA_UPSERT_ENABLED", true),         "Enable smart upsert (default: true)")
	upsertWorkers  := flag.Int("upsert-workers",   envOrInt("IA_UPSERT_WORKERS", 2),             "Async workers for post-save processing")
	upsertThreshUpdate   := flag.Float64("upsert-thresh-update",   envOrFloat64("IA_UPSERT_THRESH_UPDATE", 0.92), "Similarity threshold for update (0.92)")
	upsertThreshRelated  := flag.Float64("upsert-thresh-related",  envOrFloat64("IA_UPSERT_THRESH_RELATED", 0.70), "Similarity threshold for relation (0.70)")
	project    := flag.String("project",       envOr("IA_PROJECT", "default"),         "Default project name")
	showVer    := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *showVer {
		fmt.Println("ia-recuerdo", version)
		os.Exit(0)
	}

	embedInfo := "disabled"
	if *embedURL != "" {
		embedInfo = fmt.Sprintf("%s model=%s dims=%d", *embedURL, *embedModel, *embedDims)
	}
	log.Printf("ia-recuerdo %s starting | transport=%s addr=%s driver=%s project=%s embed=%s",
		version, *transport, *addr, *dbDriver, *project, embedInfo)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// ── Embedding provider (optional) ───────────────────────────
	var emb embedding.Provider = &embedding.Disabled{}
	if *embedURL != "" {
		emb = embedding.New(*embedURL, *embedModel, *embedToken, *embedFmt, *embedDims)
	}

	// ── Store ──────────────────────────────────────────────────────
	st, err := store.New(ctx, store.Config{
		Driver:    *dbDriver,
		DSN:       *dbDSN,
		EmbedDims: *embedDims,
		UpsertCfg: types.SmartUpsertConfig{
			Enabled:          *upsertEnabled,
			ThresholdUpdate:  *upsertThreshUpdate,
			ThresholdRelated: *upsertThreshRelated,
			AsyncWorkers:     *upsertWorkers,
		},
	}, emb)
	if err != nil {
		log.Fatalf("cannot open store: %v", err)
	}
	defer st.Close()

	// ── Cache (optional) ──────────────────────────────────────────
	ca := cache.New(*valkeyAddr)
	defer ca.Close()

	// ── MCP Handler ───────────────────────────────────────────────
	h := mcp.New(st, ca, emb)

	// ── Transport(s) ──────────────────────────────────────────────
	errCh := make(chan error, 2)

	switch *transport {
	case "stdio":
		go func() { errCh <- h.ServeStdio(ctx) }()

	case "http":
		srv := server.New(st, h, ca, *addr)
		go func() { errCh <- srv.Start(ctx) }()

	case "both":
		srv := server.New(st, h, ca, *addr)
		go func() { errCh <- h.ServeStdio(ctx) }()
		go func() { errCh <- srv.Start(ctx) }()

	default:
		log.Fatalf("unknown transport %q — use stdio, http, or both", *transport)
	}

	select {
	case <-ctx.Done():
		log.Println("shutting down gracefully")
	case err := <-errCh:
		if err != nil {
			log.Fatalf("transport error: %v", err)
		}
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envOrInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return fallback
}

func envOrBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		switch strings.ToLower(v) {
		case "true", "1", "yes", "on":
			return true
		case "false", "0", "no", "off":
			return false
		}
	}
	return fallback
}

func envOrFloat64(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		var n float64
		if _, err := fmt.Sscanf(v, "%f", &n); err == nil {
			return n
		}
	}
	return fallback
}
