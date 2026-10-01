package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Darida/openrouterclient/src/internal/replyfile"
	"github.com/Darida/openrouterclient/src/model"
)

const (
	fetchTimeout = 30 * time.Second
	cacheTTL     = time.Hour
	// OpenRouter lists routers, which aren't models, at this placeholder price.
	routerPrice = "-1"
)

// Catalog is OpenRouter's model list, fetched at most once per cacheTTL.
type Catalog struct {
	URL  string
	HTTP *http.Client
	// Describes the catalog's raw body in error messages.
	Replies replyfile.Saver

	mu        sync.Mutex
	fetchedAt time.Time
	models    []entry
	body      []byte
}

// Candidates lists tier's models that support strict json_schema output,
// skipping routers. It errors on a candidate whose price is negative or
// unparseable, or whose context length is missing.
func (c *Catalog) Candidates(tier model.ModelTier) ([]Model, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.fetchedAt) >= cacheTTL {
		if err := c.refresh(); err != nil {
			return nil, err
		}
	}
	var candidates []Model
	for _, m := range c.models {
		member, err := inTier(m.ID, tier)
		if err != nil {
			return nil, err
		}
		if !member || isBatchOnly(m.ID) || isRouter(m) || !slices.Contains(m.SupportedParameters, "structured_outputs") || !slices.Contains(m.Architecture.OutputModalities, "text") {
			continue
		}
		candidate, err := c.candidate(m)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

// AllCandidates lists the candidates of every tier.
func (c *Catalog) AllCandidates() ([]Model, error) {
	var all []Model
	for _, tier := range []model.ModelTier{model.ModelTierFree, model.ModelTierPaid} {
		candidates, err := c.Candidates(tier)
		if err != nil {
			return nil, err
		}
		all = append(all, candidates...)
	}
	return all, nil
}

func (c *Catalog) candidate(m entry) (Model, error) {
	if m.ContextLength <= 0 {
		return Model{}, c.unexpected(fmt.Sprintf("model %q has no context length", m.ID))
	}
	prompt, err := c.price(m.ID, "prompt", m.Pricing.Prompt)
	if err != nil {
		return Model{}, err
	}
	completion, err := c.price(m.ID, "completion", m.Pricing.Completion)
	if err != nil {
		return Model{}, err
	}
	return Model{ID: m.ID, ContextTokens: m.ContextLength, PromptUSDPerToken: prompt, CompletionUSDPerToken: completion}, nil
}

func (c *Catalog) refresh() error {
	body, err := c.fetch()
	if err != nil {
		return err
	}
	models, err := c.parse(body)
	if err != nil {
		return err
	}
	c.body, c.models, c.fetchedAt = body, models, time.Now()
	return nil
}

func (c *Catalog) fetch() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("catalog: build request: %w", err)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("catalog: fetch %s: %w", c.URL, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("catalog: read %s: %w", c.URL, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, c.describedError(body, fmt.Sprintf("%s returned HTTP %d", c.URL, resp.StatusCode))
	}
	return body, nil
}

func (c *Catalog) parse(body []byte) ([]entry, error) {
	var parsed struct {
		Data []entry `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || len(parsed.Data) == 0 {
		return nil, c.describedError(body, fmt.Sprintf("%s returned no models: %v", c.URL, err))
	}
	for _, m := range parsed.Data {
		if m.ID == "" {
			return nil, c.describedError(body, "model entry without id")
		}
	}
	return parsed.Data, nil
}

// unexpected describes the cached catalog body, which holds the offending entry.
func (c *Catalog) unexpected(problem string) error {
	return c.describedError(c.body, problem)
}

func (c *Catalog) describedError(body []byte, problem string) error {
	described, err := c.Replies.Describe("", "catalog", body)
	if err != nil {
		return err
	}
	return fmt.Errorf("catalog: %s: %s", problem, described)
}

// OpenRouter's own "openrouter/…" entries are routers, not models.
func inTier(id string, tier model.ModelTier) (bool, error) {
	free := strings.HasSuffix(id, ":free")
	switch tier {
	case model.ModelTierFree:
		return free, nil
	case model.ModelTierPaid:
		return !free && !strings.HasPrefix(id, "openrouter/"), nil
	}
	return false, fmt.Errorf("catalog: unknown model tier %q", tier)
}

func isRouter(m entry) bool {
	return m.Pricing.Prompt == routerPrice || m.Pricing.Completion == routerPrice
}

// A ":batch" variant rejects the chat/completions endpoint with a 404.
func isBatchOnly(id string) bool {
	return strings.HasSuffix(id, ":batch")
}

func (c *Catalog) price(id, kind, raw string) (float64, error) {
	usd, err := strconv.ParseFloat(raw, 64)
	if err != nil || usd < 0 {
		return 0, c.unexpected(fmt.Sprintf("model %q has an invalid %s price %q", id, kind, raw))
	}
	return usd, nil
}
