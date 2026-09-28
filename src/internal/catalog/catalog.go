package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	fetchTimeout = 30 * time.Second
	cacheTTL     = time.Hour
)

// Catalog is OpenRouter's model list, fetched at most once per cacheTTL.
type Catalog struct {
	URL  string
	HTTP *http.Client

	mu        sync.Mutex
	fetchedAt time.Time
	models    []entry
	byLog     map[string][]string
}

// FreeStructuredModels lists the free models that can answer with a strict
// json_schema response format, which every request here uses.
func (c *Catalog) FreeStructuredModels() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.fetchedAt) >= cacheTTL {
		c.refresh()
	}
	var ids []string
	for _, m := range c.models {
		if strings.HasSuffix(m.ID, ":free") && slices.Contains(m.SupportedParameters, "structured_outputs") && slices.Contains(m.Architecture.OutputModalities, "text") {
			ids = append(ids, m.ID)
		}
	}
	return ids
}

// ModelID translates the dated name OpenRouter's generation log uses
// ("vendor/model-20260811:free") into the model id chat responses use
// ("vendor/model:free"). A miss refetches first, since a model may be newer
// than the cache; it panics unless exactly one model matches.
func (c *Catalog) ModelID(logName string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.fetchedAt) >= cacheTTL || len(c.byLog[logName]) == 0 {
		c.refresh()
	}
	ids := c.byLog[logName]
	if len(ids) != 1 {
		panic(fmt.Sprintf("catalog: log model %q matches %d catalog models %v, want exactly 1", logName, len(ids), ids))
	}
	return ids[0]
}

func (c *Catalog) refresh() {
	models := c.fetch()
	byLog := map[string][]string{}
	for _, m := range models {
		key := logNameFor(m.ID, m.CanonicalSlug)
		byLog[key] = append(byLog[key], m.ID)
	}
	c.models, c.byLog, c.fetchedAt = models, byLog, time.Now()
}

func (c *Catalog) fetch() []entry {
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		panic(fmt.Sprintf("catalog: build request: %v", err))
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		panic(fmt.Sprintf("catalog: fetch %s: %v", c.URL, err))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(fmt.Sprintf("catalog: read %s: %v", c.URL, err))
	}
	if resp.StatusCode != http.StatusOK {
		panic(fmt.Sprintf("catalog: %s returned HTTP %d: %s", c.URL, resp.StatusCode, body))
	}
	var parsed struct {
		Data []entry `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || len(parsed.Data) == 0 {
		panic(fmt.Sprintf("catalog: %s returned no models: %v", c.URL, err))
	}
	for _, m := range parsed.Data {
		if m.ID == "" || m.CanonicalSlug == "" {
			panic(fmt.Sprintf("catalog: model entry without id or canonical_slug: %+v", m))
		}
	}
	return parsed.Data
}

// Paid and free variants share one canonical slug; the log tells them apart
// by keeping the id's ":variant" suffix.
func logNameFor(id, canonicalSlug string) string {
	if i := strings.LastIndex(id, ":"); i != -1 {
		return canonicalSlug + id[i:]
	}
	return canonicalSlug
}
