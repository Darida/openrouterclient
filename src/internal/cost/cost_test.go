package cost

import (
	"testing"

	"github.com/Darida/openrouterclient/src/internal/catalog"
)

func TestEstimateTokens_whenNineChars_thenThreeTokens(t *testing.T) {
	// Act
	got := EstimateTokens("123456789")

	// Assert
	if got != 3 {
		t.Fatalf("got %d, want 3", got)
	}
}

func TestEstimate_whenPriced_thenSumsPromptAndMaxOutput(t *testing.T) {
	// Arrange
	m := catalog.Model{ID: "m", PromptUSDPerToken: 0.001, CompletionUSDPerToken: 0.002}

	// Act
	got := Estimate(m, 100, 1000)

	// Assert
	if got != 0.1+2 {
		t.Fatalf("got %v, want 2.1", got)
	}
}

func TestCheapestPool_whenPricesSpread_thenKeepsOnlyThoseNearThe30thPercentile(t *testing.T) {
	// Arrange
	var models []catalog.Model
	for i, completion := range []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10} {
		models = append(models, catalog.Model{ID: string(rune('a' + i)), CompletionUSDPerToken: completion})
	}

	// Act
	pool, _ := CheapestPool(models, 0, 1)

	// Assert
	if len(pool) != 4 {
		t.Fatalf("pool = %v; the 30th percentile is 4, ×1.1 = 4.4, so a–d should remain", pool)
	}
}
