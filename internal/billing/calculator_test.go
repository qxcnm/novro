package billing

import (
	"math"
	"testing"
)

func TestCalculateCostUsesAllTokenDimensionsAndMultiplier(t *testing.T) {
	tokens := TokenBreakdown{UncachedInput: 500_000, CacheRead: 250_000, CacheWrite: 100_000, Output: 200_000}
	rates := RateCard{InputMicros: 1_000_000, CacheReadMicros: 200_000, CacheWriteMicros: 1_250_000, OutputMicros: 3_000_000, RequestMicros: 1_000}
	quote, err := CalculateCost(tokens, rates, 12_000)
	if err != nil {
		t.Fatalf("calculate: %v", err)
	}
	if quote.BaseCostMicros != 1_276_000 || quote.CostMicros != 1_531_200 {
		t.Fatalf("unexpected quote: %+v", quote)
	}
}

func TestCalculateCostRoundsOnceAfterSummingComponents(t *testing.T) {
	quote, err := CalculateCost(TokenBreakdown{UncachedInput: 1, CacheRead: 1, CacheWrite: 1, Output: 1}, RateCard{InputMicros: 100_000, CacheReadMicros: 100_000, CacheWriteMicros: 100_000, OutputMicros: 100_000}, 10_000)
	if err != nil {
		t.Fatalf("calculate: %v", err)
	}
	if quote.BaseCostMicros != 1 || quote.CostMicros != 1 {
		t.Fatalf("rounding must happen once, got %+v", quote)
	}
}

func TestCalculateCostRoundsFinalCustomerChargeUp(t *testing.T) {
	quote, err := CalculateCost(TokenBreakdown{UncachedInput: 1}, RateCard{InputMicros: 20_000}, 15_000)
	if err != nil {
		t.Fatalf("calculate: %v", err)
	}
	if quote.BaseCostMicros != 1 || quote.CostMicros != 1 {
		t.Fatalf("unexpected tiny charge: %+v", quote)
	}
}

func TestEstimateReservationUsesHighestInputDimensionRate(t *testing.T) {
	quote, err := EstimateReservation(100, 10, RateCard{InputMicros: 1_000_000, CacheReadMicros: 100_000, CacheWriteMicros: 1_250_000, CacheWrite1hMicros: 2_000_000, OutputMicros: 5_000_000}, 10_000)
	if err != nil {
		t.Fatalf("estimate: %v", err)
	}
	if quote.CostMicros != 250 {
		t.Fatalf("unexpected reservation: %+v", quote)
	}
}

func TestEstimateReservationAppliesBillingGroupMultiplier(t *testing.T) {
	quote, err := EstimateReservation(1_000_000, 0, RateCard{InputMicros: 3_000_000}, 4_000)
	if err != nil {
		t.Fatalf("estimate: %v", err)
	}
	if quote.BaseCostMicros != 3_000_000 || quote.CostMicros != 1_200_000 {
		t.Fatalf("0.4 multiplier must reduce a 3 yuan reservation to 1.2 yuan: %+v", quote)
	}
}

func TestEstimateReservationCoversEveryActualInputDimension(t *testing.T) {
	rates := RateCard{InputMicros: 1_000_000, CacheReadMicros: 100_000, CacheWriteMicros: 1_250_000, CacheWrite1hMicros: 2_000_000, OutputMicros: 5_000_000, RequestMicros: 7}
	reservation, err := EstimateReservation(100, 20, rates, 12_500)
	if err != nil {
		t.Fatalf("estimate: %v", err)
	}
	for _, tokens := range []TokenBreakdown{
		{UncachedInput: 100, Output: 20},
		{CacheRead: 100, Output: 20},
		{CacheWrite: 100, Output: 20},
		{CacheWrite1h: 100, Output: 20},
		{UncachedInput: 25, CacheRead: 25, CacheWrite: 25, CacheWrite1h: 25, Output: 20},
	} {
		actual, err := CalculateCost(tokens, rates, 12_500)
		if err != nil {
			t.Fatalf("calculate %+v: %v", tokens, err)
		}
		if reservation.CostMicros < actual.CostMicros {
			t.Fatalf("reservation %d does not cover %+v cost %d", reservation.CostMicros, tokens, actual.CostMicros)
		}
	}
}

func TestCalculateCostRejectsDatabaseTokenOverflow(t *testing.T) {
	for _, tokens := range []TokenBreakdown{
		{UncachedInput: math.MaxInt32, CacheRead: 1},
		{Output: int(MaxRecordedTokens) + 1},
	} {
		if _, err := CalculateCost(tokens, RateCard{InputMicros: 1}, 10_000); err == nil {
			t.Fatalf("expected overflow rejection for %+v", tokens)
		}
	}
}
