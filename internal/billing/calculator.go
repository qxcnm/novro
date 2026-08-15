package billing

import (
	"math/big"
)

const (
	// 价格以“每百万 Token 的人民币微元”保存。1 元 = 1,000,000 微元，
	// 因而 Token 成本的分子是 tokenCount * priceMicros，除以该常量后才是微元。
	PriceUnitTokens int64 = 1_000_000
	// 倍率以基点表示：10,000 为原价，12,500 为 1.25 倍，4,000 为 0.4 倍。
	BasisPointsUnit int64 = 10_000
	// MySQL 的 Token 字段使用有符号 INT；输入各维度之和也必须落在同一范围内。
	MaxRecordedTokens  int64 = 2_147_483_647
	CalculationVersion       = "token-v3-confirmed-usage"
)

/**
 * TokenBreakdown 表示一次调用中彼此互斥的计费 Token 维度。
 * 普通输入、缓存命中、5 分钟缓存创建、1 小时缓存创建和输出分别按独立单价结算。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-15
 */
type TokenBreakdown struct {
	UncachedInput int `json:"uncached_input_tokens"`
	CacheRead     int `json:"cache_read_input_tokens"`
	CacheWrite    int `json:"cache_write_input_tokens"`
	CacheWrite1h  int `json:"cache_write_1h_input_tokens"`
	Output        int `json:"output_tokens"`
}

/**
 * InputTotal 汇总所有输入维度，不包含输出 Token。
 * @param none 无参数。
 * @return 用于审计记录的输入 Token 总数。
 * @author Gao Hongshun
 * @date 2026-08-15
 */
func (t TokenBreakdown) InputTotal() int {
	return t.UncachedInput + t.CacheRead + t.CacheWrite + t.CacheWrite1h
}

/**
 * RateCard 保存一次结算所需的六维费率，单位均为人民币微元。
 * 前五项是每百万 Token 的价格，RequestMicros 是每次请求直接加入的固定费用。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-15
 */
type RateCard struct {
	InputMicros        int64 `json:"input_price_micros"`
	OutputMicros       int64 `json:"output_price_micros"`
	CacheReadMicros    int64 `json:"cache_read_price_micros"`
	CacheWriteMicros   int64 `json:"cache_write_price_micros"`
	CacheWrite1hMicros int64 `json:"cache_write_1h_price_micros"`
	RequestMicros      int64 `json:"request_price_micros"`
}

/**
 * Quote 同时保存未应用分组倍率的基础成本和用户实际应付成本。
 * 两个值都以人民币微元表示；保留基础成本可让历史流水解释倍率带来的价格差异。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-15
 */
type Quote struct {
	BaseCostMicros int64
	CostMicros     int64
}

/**
 * CalculateCost 根据实际 Token 明细和固定费率计算基础成本及用户应付成本。
 * 先计算分子 N = Σ(Token维度 * 每百万Token单价) + 请求固定费 * 1,000,000；
 * 基础成本为 ceil(N / 1,000,000)，应付成本为 ceil(N * 倍率基点 / (1,000,000 * 10,000))。
 * 两个结果都在全部维度汇总后才向上取整，避免每一项单独取整造成累计多扣。
 * 使用 big.Int 是因为合法的 Token 上限和费率相乘可能超过 int64，即使最终微元金额仍可表示。
 * @param tokens 上游确认的五维 Token 明细。
 * @param rates 本次请求开始时固定下来的六维费率。
 * @param multiplierBPS 计费分组倍率，10,000 表示原价。
 * @return 基础成本、应用倍率后的成本或非法输入错误。
 * @author Gao Hongshun
 * @date 2026-08-15
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
	// 固定请求费没有 Token 分母；乘回 PriceUnitTokens 后可与所有 Token 项在同一分子相加。
	numerator.Add(numerator, new(big.Int).Mul(big.NewInt(rates.RequestMicros), big.NewInt(PriceUnitTokens)))
	base, ok := ceilQuotient(numerator, big.NewInt(PriceUnitTokens))
	if !ok {
		return Quote{}, ErrInvalidInput
	}
	// 直接对原始分子应用倍率，而非先把 base 向上取整再乘倍率，保证只有一次舍入。
	chargedNumerator := new(big.Int).Mul(numerator, big.NewInt(multiplierBPS))
	charged, ok := ceilQuotient(chargedNumerator, big.NewInt(PriceUnitTokens*BasisPointsUnit))
	if !ok {
		return Quote{}, ErrInvalidInput
	}
	return Quote{BaseCostMicros: base, CostMicros: charged}, nil
}

/**
 * EstimateReservation 使用请求开始前可得的上限估算需要冻结的余额。
 * 上游尚未返回 usage 时无法判断输入会落入哪一种缓存维度，因此将全部输入按四种输入单价中最高者计算；
 * 这样任意实际缓存拆分的最终费用都不会高于预占费用。输出使用客户端声明或默认的最大输出 Token。
 * 最终结算只能使用上游确认 usage，不能把本估算值当成实际费用。
 * @param inputTokens 已按网关上限截断的输入 Token 估算。
 * @param outputTokens 已按网关上限截断的最大输出 Token。
 * @param rates 本次请求固定的六维费率。
 * @param multiplierBPS 计费分组倍率，10,000 表示原价。
 * @return 预占对应的基础成本和用户成本或非法输入错误。
 * @author Gao Hongshun
 * @date 2026-08-15
 */
func EstimateReservation(inputTokens, outputTokens int, rates RateCard, multiplierBPS int64) (Quote, error) {
	// 预占阶段只有总输入估算，不可假设缓存命中；取最高费率保证预占覆盖所有真实输入维度。
	inputRate := max(rates.InputMicros, rates.CacheReadMicros, rates.CacheWriteMicros, rates.CacheWrite1hMicros)
	rates.InputMicros = inputRate
	return CalculateCost(TokenBreakdown{UncachedInput: inputTokens, Output: outputTokens}, rates, multiplierBPS)
}

/**
 * addTokenCost 将一个按百万 Token 定价的维度加入公共分子。
 * 此处刻意不除以 PriceUnitTokens，所有维度汇总后由 CalculateCost 统一向上取整。
 * @param total 以“Token乘以微元”计量的累计分子。
 * @param tokens 当前维度已确认或预估的 Token 数。
 * @param rate 当前维度每百万 Token 的微元单价。
 * @author Gao Hongshun
 * @date 2026-08-15
 */
func addTokenCost(total *big.Int, tokens int, rate int64) {
	if tokens == 0 || rate == 0 {
		return
	}
	total.Add(total, new(big.Int).Mul(big.NewInt(int64(tokens)), big.NewInt(rate)))
}

/**
 * ceilQuotient 计算正整数除法的向上取整，并确保结果仍可安全转换为 int64。
 * 对正分子使用 (numerator + denominator - 1) / denominator，零分子保持零。
 * @param numerator 非负整数分子。
 * @param denominator 正整数分母。
 * @return 向上取整后的 int64 结果和是否未溢出。
 * @author Gao Hongshun
 * @date 2026-08-15
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
 * validBreakdown 校验 Token 明细可以安全写入数据库，并保持输入总数与持久化字段一致。
 * 输出单独受上限约束；四种输入维度各自合法但总和溢出同样必须拒绝。
 * @param tokens 待计费的五维 Token 明细。
 * @return 明细和输入总数是否都在数据库可表示范围内。
 * @author Gao Hongshun
 * @date 2026-08-15
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
 * validRates 校验六维费率为可计算的非负微元整数。
 * @param rates 待用于预占或结算的费率卡。
 * @return 所有维度是否在系统允许的价格范围内。
 * @author Gao Hongshun
 * @date 2026-08-15
 */
func validRates(rates RateCard) bool {
	for _, value := range []int64{rates.InputMicros, rates.OutputMicros, rates.CacheReadMicros, rates.CacheWriteMicros, rates.CacheWrite1hMicros, rates.RequestMicros} {
		if value < 0 || value > 1_000_000_000_000 {
			return false
		}
	}
	return true
}
