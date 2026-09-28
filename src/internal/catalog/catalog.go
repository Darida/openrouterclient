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

func (c *Catalog) refresh() {
	c.models, c.fetchedAt = c.fetch(), time.Now()
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
		if m.ID == "" {
			panic(fmt.Sprintf("catalog: model entry without id: %+v", m))
		}
	}
	return parsed.Data
}
