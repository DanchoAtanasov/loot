package items

import (
	"testing"
)

func TestDropItemTierRates(t *testing.T) {
	const simulations = 100_000
	counts := make(map[ItemTier]int)

	for i := 0; i < simulations; i++ {
		counts[DropItem().Tier]++
	}

	// rollTier checks each tier in order, with NormalTier as the fallback.
	// These are the resulting probabilities after the cascading rolls.
	want := map[ItemTier]float64{
		UniqueTier: 2,
		RareTier:   0.98 * 8,
		MagicTier:  0.98 * 0.92 * 20,
		NormalTier: 100 - 2 - (0.98 * 8) - (0.98 * 0.92 * 20),
	}
	const tolerancePercent = 1.0

	for tier, expectedPercent := range want {
		gotPercent := float64(counts[tier]) / simulations * 100
		if difference := gotPercent - expectedPercent; difference < -tolerancePercent || difference > tolerancePercent {
			t.Errorf("%v drop rate = %.2f%%, want %.2f%% (±%.1f%%)", tier, gotPercent, expectedPercent, tolerancePercent)
		}
	}
}
