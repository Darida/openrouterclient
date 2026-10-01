package cost

import (
	"errors"
	"math"
	"sort"

	"github.com/Darida/openrouterclient/src/internal/catalog"
)

// assetloom's rule: keep models priced within 10% of the 30th-percentile
// estimate, then pick at random, spreading load instead of always the cheapest.
const (
	poolPercentile = 0.3
	poolHeadroom   = 1.1
)

// EstimateTokens uses ~4 chars per token. This ranks models; it doesn't bill.
func EstimateTokens(text string) int {
	return int(math.Ceil(float64(len(text)) / 4))
}

// Estimate prices a request as if it used every one of maxOutputTokens.
func Estimate(m catalog.Model, promptTokens, maxOutputTokens int) float64 {
	return m.PromptUSDPerToken*float64(promptTokens) + m.CompletionUSDPerToken*float64(maxOutputTokens)
}

// CheapestPool keeps the models whose estimate is at most poolHeadroom times
// the poolPercentile estimate among models. It errors on an empty models.
func CheapestPool(models []catalog.Model, promptTokens, maxOutputTokens int) (pool []catalog.Model, ceilingUSD float64, err error) {
	if len(models) == 0 {
		return nil, 0, errors.New("cost: no models to price")
	}
	estimates := make([]float64, len(models))
	for i, m := range models {
		estimates[i] = Estimate(m, promptTokens, maxOutputTokens)
	}
	sorted := append([]float64(nil), estimates...)
	sort.Float64s(sorted)
	ceilingUSD = percentile(sorted, poolPercentile) * poolHeadroom
	for i, m := range models {
		if estimates[i] <= ceilingUSD {
			pool = append(pool, m)
		}
	}
	return pool, ceilingUSD, nil
}

// Nearest-rank, no interpolation: the threshold is a real observed estimate.
func percentile(sortedAscending []float64, p float64) float64 {
	index := min(len(sortedAscending)-1, int(math.Floor(p*float64(len(sortedAscending)))))
	return sortedAscending[index]
}
