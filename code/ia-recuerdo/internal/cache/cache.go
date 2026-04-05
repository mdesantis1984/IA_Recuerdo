// Package cache wraps Valkey (Redis-compatible) for caching search results and session context.
package cache

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/valkey-io/valkey-go"
)

const (
	DefaultSearchTTL  = 5 * time.Minute
	DefaultContextTTL = 1 * time.Hour
)

// Cache wraps a Valkey client.
type Cache struct {
	client valkey.Client
}

// New creates a new Cache. If addr is empty, returns a no-op cache.
func New(addr string) *Cache {
	if addr == "" {
		log.Println("[cache] No Valkey address — running without cache")
		return &Cache{}
	}
	c, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{addr},
	})
	if err != nil {
		log.Printf("[cache] Failed to connect to Valkey at %s: %v — running without cache", addr, err)
		return &Cache{}
	}
	log.Printf("[cache] Connected to Valkey at %s", addr)
	return &Cache{client: c}
}

func (c *Cache) enabled() bool { return c.client != nil }

// Close releases the Valkey connection.
func (c *Cache) Close() {
	if c.enabled() {
		c.client.Close()
	}
}

// GetSearch retrieves a cached search result set by query fingerprint.
func (c *Cache) GetSearch(ctx context.Context, fp string, dest interface{}) bool {
	if !c.enabled() {
		return false
	}
	cmd := c.client.B().Get().Key("search:" + fp).Build()
	result := c.client.Do(ctx, cmd)
	if result.Error() != nil {
		return false
	}
	raw, err := result.AsBytes()
	if err != nil {
		return false
	}
	return json.Unmarshal(raw, dest) == nil
}

// SetSearch caches a search result set.
func (c *Cache) SetSearch(ctx context.Context, fp string, data interface{}) {
	if !c.enabled() {
		return
	}
	b, err := json.Marshal(data)
	if err != nil {
		return
	}
	cmd := c.client.B().Set().Key("search:"+fp).Value(string(b)).Ex(DefaultSearchTTL).Build()
	_ = c.client.Do(ctx, cmd)
}

// GetContext retrieves cached session context.
func (c *Cache) GetContext(ctx context.Context, project string, dest interface{}) bool {
	if !c.enabled() {
		return false
	}
	cmd := c.client.B().Get().Key("ctx:" + project).Build()
	result := c.client.Do(ctx, cmd)
	if result.Error() != nil {
		return false
	}
	raw, err := result.AsBytes()
	if err != nil {
		return false
	}
	return json.Unmarshal(raw, dest) == nil
}

// SetContext caches session context for a project.
func (c *Cache) SetContext(ctx context.Context, project string, data interface{}) {
	if !c.enabled() {
		return
	}
	b, err := json.Marshal(data)
	if err != nil {
		return
	}
	cmd := c.client.B().Set().Key("ctx:"+project).Value(string(b)).Ex(DefaultContextTTL).Build()
	_ = c.client.Do(ctx, cmd)
}

// InvalidateContext removes cached context for a project (call after mem_save).
func (c *Cache) InvalidateContext(ctx context.Context, project string) {
	if !c.enabled() {
		return
	}
	cmd := c.client.B().Del().Key("ctx:" + project).Build()
	_ = c.client.Do(ctx, cmd)
}
