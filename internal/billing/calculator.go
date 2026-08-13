package billing

import (
	"math/big"
)

const (
	PriceUnitTokens    int64 = 1_000_000
	BasisPointsUnit    int64 = 10_000
	MaxRecordedTokens  int64 = 2_147_483_647
	CalculationVersion       = "token-v3-confirmed-usage"
)

type TokenBreakdown struct {
	UncachedInput int `json:"uncached_input_tokens"`
	CacheRead     int `json:"cache_read_input_tokens"`
	CacheWrite    int `json:"cache_write_input_tokens"`
	CacheWrite1h  int `json:"cache_write_1h_input_tokens"`
	Output        int `json:"output_tokens"`
}

/**
 * InputTotal 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (t TokenBreakdown) InputTotal() int {
	return t.UncachedInput + t.CacheRead + t.CacheWrite + t.CacheWrite1h
}

type RateCard struct {
	InputMicros        int64 `json:"input_price_micros"`
	OutputMicros       int64 `json:"output_price_micros"`
	CacheReadMicros    int64 `json:"cache_read_price_micros"`
	CacheWriteMicros   int64 `json:"cache_write_price_micros"`
	CacheWrite1hMicros int64 `json:"cache_write_1h_price_micros"`
	RequestMicros      int64 `json:"request_price_micros"`
}

type Quote struct {
	BaseCostMicros int64
	CostMicros     int64
}

/**
 * CalculateCost 用于计算并返回对应结果。
 * @param tokens 本次操作需要使用的输入参数。
 * @param rates 本次操作需要使用的输入参数。
 * @param multiplierBPS 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func CalculateCost(tokens TokenBreakdown, rates RateCard, multiplierBPS int64) (Quote, error) {
	if !validBreakdown(tokens) || !validRates(rates) || multiplierBPS < 1 || multiplierBPS > 1_000_000 {
		return Quote{}, ErrInvalidInput
	}
	numerator := new(big.Int)
	addTokenCost(numerator, tokens.UncachedInput, rates.InputMicros)
	addTokenCost(numerator, tokens.CacheRead, rates.CacheReadMicros)
	addTokenCost(numerator, tokens.CacheWrite, rates.CacheWriteMicros)
	addTokenCost(numerator, tokens.CacheWrite1h, rates.CacheWrite1hMicros)
	addTokenCost(numerator, tokens.Output, rates.OutputMicros)
	numerator.Add(numerator, new(big.Int).Mul(big.NewInt(rates.RequestMicros), big.NewInt(PriceUnitTokens)))
	base, ok := ceilQuotient(numerator, big.NewInt(PriceUnitTokens))
	if !ok {
		return Quote{}, ErrInvalidInput
	}
	chargedNumerator := new(big.Int).Mul(numerator, big.NewInt(multiplierBPS))
	charged, ok := ceilQuotient(chargedNumerator, big.NewInt(PriceUnitTokens*BasisPointsUnit))
	if !ok {
		return Quote{}, ErrInvalidInput
	}
	return Quote{BaseCostMicros: base, CostMicros: charged}, nil
}

/**
 * EstimateReservation 封装该名称对应的业务处理逻辑。
 * @param inputTokens 本次操作需要使用的输入参数。
 * @param outputTokens 本次操作需要使用的输入参数。
 * @param rates 本次操作需要使用的输入参数。
 * @param multiplierBPS 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func EstimateReservation(inputTokens, outputTokens int, rates RateCard, multiplierBPS int64) (Quote, error) {
	inputRate := max(rates.InputMicros, rates.CacheReadMicros, rates.CacheWriteMicros, rates.CacheWrite1hMicros)
	rates.InputMicros = inputRate
	return CalculateCost(TokenBreakdown{UncachedInput: inputTokens, Output: outputTokens}, rates, multiplierBPS)
}

/**
 * addTokenCost 封装该名称对应的业务处理逻辑。
 * @param total 本次操作需要使用的输入参数。
 * @param tokens 本次操作需要使用的输入参数。
 * @param rate 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func addTokenCost(total *big.Int, tokens int, rate int64) {
	if tokens == 0 || rate == 0 {
		return
	}
	total.Add(total, new(big.Int).Mul(big.NewInt(int64(tokens)), big.NewInt(rate)))
}

/**
 * ceilQuotient 封装该名称对应的业务处理逻辑。
 * @param numerator 本次操作需要使用的输入参数。
 * @param denominator 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func ceilQuotient(numerator, denominator *big.Int) (int64, bool) {
	if numerator.Sign() == 0 {
		return 0, true
	}
	adjusted := new(big.Int).Add(new(big.Int).Set(numerator), new(big.Int).Sub(new(big.Int).Set(denominator), big.NewInt(1)))
	result := adjusted.Quo(adjusted, denominator)
	return result.Int64(), result.IsInt64()
}

/**
 * validBreakdown 封装该名称对应的业务处理逻辑。
 * @param tokens 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func validBreakdown(tokens TokenBreakdown) bool {
	values := []int{tokens.UncachedInput, tokens.CacheRead, tokens.CacheWrite, tokens.CacheWrite1h, tokens.Output}
	var inputTotal int64
	for index, value := range values {
		if value < 0 || int64(value) > MaxRecordedTokens {
			return false
		}
		if index < len(values)-1 {
			inputTotal += int64(value)
		}
	}
	return inputTotal <= MaxRecordedTokens
}

/**
 * validRates 封装该名称对应的业务处理逻辑。
 * @param rates 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func validRates(rates RateCard) bool {
	for _, value := range []int64{rates.InputMicros, rates.OutputMicros, rates.CacheReadMicros, rates.CacheWriteMicros, rates.CacheWrite1hMicros, rates.RequestMicros} {
		if value < 0 || value > 1_000_000_000_000 {
			return false
		}
	}
	return true
}
