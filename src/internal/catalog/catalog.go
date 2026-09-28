package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const fetchTimeout = 30 * time.Second

// Catalog translates the dated model names OpenRouter's generation log uses
// (canonical slug plus variant, e.g. "vendor/model-20260811:free") into the
// model ids chat responses use ("vendor/model:free").
type Catalog struct {
	URL  string
	HTTP *http.Client

	mu    sync.Mutex
	byLog map[string][]string
}

// ModelID panics if logName matches no model, or more than one, even after
// refreshing the catalog, since a new model may have appeared since the last fetch.
func (c *Catalog) ModelID(logName string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.byLog == nil || len(c.byLog[logName]) == 0 {
		c.byLog = c.fetch()
	}
	ids := c.byLog[logName]
	if len(ids) != 1 {
		panic(fmt.Sprintf("catalog: log model %q matches %d catalog models %v, want exactly 1", logName, len(ids), ids))
	}
	return ids[0]
}

func (c *Catalog) fetch() map[string][]string {
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
		Data []struct {
			ID            string `json:"id"`
			CanonicalSlug string `json:"canonical_slug"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || len(parsed.Data) == 0 {
		panic(fmt.Sprintf("catalog: %s returned no models: %v", c.URL, err))
	}
	byLog := map[string][]string{}
	for _, m := range parsed.Data {
		if m.ID == "" || m.CanonicalSlug == "" {
			panic(fmt.Sprintf("catalog: model entry without id or canonical_slug: %+v", m))
		}
		key := logNameFor(m.ID, m.CanonicalSlug)
		byLog[key] = append(byLog[key], m.ID)
	}
	return byLog
}

// Paid and free variants share one canonical slug; the log tells them apart
// by keeping the id's ":variant" suffix.
func logNameFor(id, canonicalSlug string) string {
	if i := strings.LastIndex(id, ":"); i != -1 {
		return canonicalSlug + id[i:]
	}
	return canonicalSlug
}
