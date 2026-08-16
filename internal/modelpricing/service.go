package modelpricing

import (
	"context"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/internal/billing"
)

/**
 * Store 定义模型价格方案的持久化能力边界。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
type Store interface {
	List(context.Context, uuid.UUID) ([]Plan, error)
	CreateDraft(context.Context, uuid.UUID, PlanInput) (Plan, error)
	UpdateDraft(context.Context, uuid.UUID, PlanInput) (Plan, error)
	Publish(context.Context, uuid.UUID) (Plan, error)
	Republish(context.Context, uuid.UUID, time.Time) (RepublishResult, error)
	DeleteDraft(context.Context, uuid.UUID) error
	Resolve(context.Context, uuid.UUID, time.Time) (Resolution, error)
}

/**
 * pricingSnapshot 保存单个模型的完整已发布价格方案快照，供并发请求共享读取。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
type pricingSnapshot struct {
	// ready 用于让同一模型的并发请求等待首次加载完成，避免重复查询数据库。
	ready chan struct{}
	// plans 只保存已发布方案；加载完成后作为只读数据使用。
	plans []Plan
	// err 保存首次加载结果，等待者必须看到与加载者相同的结果。
	err error
}

/**
 * Service 提供模型价格方案管理和请求时价格解析能力。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
type Service struct {
	store Store

	// cache 是进程内共享快照；发布成功后按模型原子淘汰并重新加载。
	cacheMu sync.Mutex
	cache   map[uuid.UUID]*pricingSnapshot
}

/**
 * NewService 创建模型价格服务及其进程内快照缓存。
 * @param store 用于持久化和查询价格方案的存储实现。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func NewService(store Store) *Service {
	return &Service{store: store, cache: make(map[uuid.UUID]*pricingSnapshot)}
}

/**
 * List 查询模型的全部价格方案，供管理端版本列表使用。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param modelID 上游模型的唯一标识。
 * @return 价格方案列表或业务错误。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func (s *Service) List(ctx context.Context, modelID uuid.UUID) ([]Plan, error) {
	if modelID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	return s.store.List(ctx, modelID)
}

/**
 * CreateDraft 校验并创建价格方案草稿，不改变线上已发布快照。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param modelID 上游模型的唯一标识。
 * @param input 价格方案草稿内容。
 * @return 创建后的价格方案或业务错误。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func (s *Service) CreateDraft(ctx context.Context, modelID uuid.UUID, input PlanInput) (Plan, error) {
	if modelID == uuid.Nil || !normalizeAndValidate(&input) {
		return Plan{}, ErrInvalidInput
	}
	return s.store.CreateDraft(ctx, modelID, input)
}

/**
 * UpdateDraft 校验并替换价格方案草稿及其全部窗口。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param planID 价格方案的唯一标识。
 * @param input 更新后的价格方案内容。
 * @return 更新后的价格方案或业务错误。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func (s *Service) UpdateDraft(ctx context.Context, planID uuid.UUID, input PlanInput) (Plan, error) {
	if planID == uuid.Nil || !normalizeAndValidate(&input) {
		return Plan{}, ErrInvalidInput
	}
	return s.store.UpdateDraft(ctx, planID, input)
}

/**
 * Publish 发布价格方案，并在事务提交后替换对应模型的内存快照。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param planID 待发布价格方案的唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func (s *Service) Publish(ctx context.Context, planID uuid.UUID) (Plan, error) {
	if planID == uuid.Nil {
		return Plan{}, ErrInvalidInput
	}
	// 发布事务和快照替换共用同一把锁，确保请求只能看到完整的旧版本或新版本。
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	plan, err := s.store.Publish(ctx, planID)
	if err == nil {
		s.invalidateLocked(plan.UpstreamModelID)
	}
	return plan, err
}

/**
 * Republish 切换到历史价格版本的生效区间，不创建新的价格版本。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param planID 要切换到的历史价格版本 ID。
 * @return 切换结果；Created 为兼容字段，始终为 false。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func (s *Service) Republish(ctx context.Context, planID uuid.UUID) (RepublishResult, error) {
	if planID == uuid.Nil {
		return RepublishResult{}, ErrInvalidInput
	}
	// 切换和普通发布共用缓存锁，避免切换过程中网关读到半套价格版本。
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	result, err := s.store.Republish(ctx, planID, time.Now().UTC())
	if err == nil {
		s.invalidateLocked(result.Plan.UpstreamModelID)
	}
	return result, err
}

/**
 * DeleteDraft 删除草稿及其窗口，不影响已发布价格方案。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param planID 待删除价格方案的唯一标识。
 * @return 删除结果或业务错误。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func (s *Service) DeleteDraft(ctx context.Context, planID uuid.UUID) error {
	if planID == uuid.Nil {
		return ErrInvalidInput
	}
	return s.store.DeleteDraft(ctx, planID)
}

/**
 * Resolve 按请求开始时间解析模型费率，并从共享快照读取已发布方案。
 * 费率只在网关开始调用上游前解析一次；调用跨过峰谷边界或其间发布新版本，都不会改变该请求的预占和最终结算价格。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param modelID 上游模型的唯一标识。
 * @param at 本次请求开始时间，使用价格方案配置的时区参与窗口判断。
 * @return 固定费率及其价格版本、窗口来源，或无有效价格错误。
 * @author Gao Hongshun
 * @date 2026-08-15
 */
func (s *Service) Resolve(ctx context.Context, modelID uuid.UUID, at time.Time) (Resolution, error) {
	if modelID == uuid.Nil || at.IsZero() {
		return Resolution{}, ErrInvalidInput
	}
	at = at.UTC()
	snapshot, err := s.snapshot(ctx, modelID)
	if err != nil {
		return Resolution{}, err
	}
	if len(snapshot.plans) == 0 {
		// 保持旧价格兼容路径不变；当前流程创建的模型都会有已发布方案，
		// 这里仅处理历史数据或滚动迁移期间尚未建立方案的模型。
		return s.store.Resolve(ctx, modelID, at)
	}
	// snapshot 内的数据只读；解析过程不持锁，避免每次模型调用都串行化在缓存锁上。
	return resolvePublishedPlans(snapshot.plans, at)
}

/**
 * snapshot 获取或加载单个模型的已发布价格快照。
 * ready 通道构成按 modelID 去重的 singleflight：第一个请求负责查库，其余请求等待同一个结果，
 * 从而避免热门模型的并发首请求同时穿透缓存。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param modelID 上游模型的唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func (s *Service) snapshot(ctx context.Context, modelID uuid.UUID) (*pricingSnapshot, error) {
	s.cacheMu.Lock()
	if entry, ok := s.cache[modelID]; ok {
		ready := entry.ready
		s.cacheMu.Unlock()
		select {
		case <-ready:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return entry, entry.err
	}
	// 在释放锁前先放入未就绪 entry，后续同模型请求会等待它而不会重复加载。
	entry := &pricingSnapshot{ready: make(chan struct{})}
	s.cache[modelID] = entry
	s.cacheMu.Unlock()

	// 数据库查询不在互斥锁中执行，避免一个慢查询阻塞其他模型的快照命中。
	plans, err := s.store.List(ctx, modelID)
	if err == nil {
		entry.plans = clonePublishedPlans(plans)
	}
	s.cacheMu.Lock()
	entry.err = err
	if err != nil {
		delete(s.cache, modelID)
	}
	close(entry.ready)
	s.cacheMu.Unlock()
	return entry, err
}

/**
 * invalidateLocked 在缓存锁内淘汰指定模型的旧快照。
 * @param modelID 上游模型的唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func (s *Service) invalidateLocked(modelID uuid.UUID) {
	if modelID == uuid.Nil {
		return
	}
	for cachedModelID := range s.cache {
		if cachedModelID == modelID {
			delete(s.cache, cachedModelID)
		}
	}
}

/**
 * clonePublishedPlans 复制已发布方案，避免存储层或请求侧修改缓存中的数据。
 * @param plans 待筛选和复制的价格方案列表。
 * @return 仅包含已发布方案的独立副本。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func clonePublishedPlans(plans []Plan) []Plan {
	cloned := make([]Plan, 0, len(plans))
	for _, plan := range plans {
		if plan.Status != StatusPublished {
			continue
		}
		if plan.EffectiveTo != nil {
			effectiveTo := *plan.EffectiveTo
			plan.EffectiveTo = &effectiveTo
		}
		plan.Windows = append([]Window(nil), plan.Windows...)
		cloned = append(cloned, plan)
	}
	return cloned
}

/**
 * resolvePublishedPlans 在内存快照中选择生效版本和峰谷窗口。
 * 版本使用 [EffectiveFrom, EffectiveTo) 的半开区间：开始时刻属于新版本，结束时刻不属于旧版本，
 * 因此相邻版本可无歧义地在同一时间点切换。
 * @param plans 已发布价格方案快照。
 * @param at 本次请求开始时间，已统一为 UTC。
 * @return 本次请求应使用的费率解析结果。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func resolvePublishedPlans(plans []Plan, at time.Time) (Resolution, error) {
	var selected *Plan
	for index := range plans {
		plan := &plans[index]
		// !at.Before(EffectiveTo) 等价于 at >= EffectiveTo，体现结束时间是排他的。
		if plan.EffectiveFrom.After(at) || (plan.EffectiveTo != nil && !at.Before(*plan.EffectiveTo)) {
			continue
		}
		if selected == nil || plan.EffectiveFrom.After(selected.EffectiveFrom) || (plan.EffectiveFrom.Equal(selected.EffectiveFrom) && plan.Version > selected.Version) {
			selected = plan
		}
	}
	if selected == nil {
		return Resolution{}, ErrNoPrice
	}
	location, err := time.LoadLocation(selected.Timezone)
	if err != nil {
		return Resolution{}, err
	}
	local := at.In(location)
	// 窗口按方案所在地的周几和当日分钟定义，而不是按服务器时区或 UTC 定义。
	minute := local.Hour()*60 + local.Minute()
	weekdayMask := 1 << int(local.Weekday())
	planID := selected.ID
	resolution := Resolution{
		Rates:         selected.DefaultRates,
		PlanID:        &planID,
		PlanVersion:   selected.Version,
		Timezone:      selected.Timezone,
		EffectiveFrom: selected.EffectiveFrom,
	}
	for index := range selected.Windows {
		window := &selected.Windows[index]
		// [StartMinute, EndMinute) 使 09:00-10:00 与 10:00-11:00 能首尾相接而不重叠。
		if window.WeekdayMask&weekdayMask == 0 || minute < window.StartMinute || minute >= window.EndMinute {
			continue
		}
		resolution.Rates = window.Rates
		windowID := window.ID
		resolution.WindowID = &windowID
		resolution.WindowLabel = window.Label
		// normalizeAndValidate 已禁止同一工作日窗口重叠，所以第一个命中窗口就是唯一答案。
		break
	}
	return resolution, nil
}

/**
 * normalizeAndValidate 规范化计价模式和时区，并校验价格窗口不重叠。
 * 窗口不跨日；跨日价格由两个相邻窗口表达。这样每个时刻只需比较当日分钟，无需处理午夜回绕。
 * @param input 待规范化和校验的价格方案输入，会在成功前被标准化。
 * @return 输入是否满足价格方案业务约束。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func normalizeAndValidate(input *PlanInput) bool {
	if input.Mode != ModeFixed && input.Mode != ModeScheduled {
		return false
	}
	if !validRates(input.DefaultRates) {
		return false
	}
	// Model price versions are never scheduled for a future publication. The
	// store replaces this sentinel with the transaction's publication time.
	input.EffectiveFrom = time.Unix(0, 0).UTC()
	input.EffectiveTo = nil
	if input.Mode == ModeFixed {
		if len(input.Windows) != 0 {
			return false
		}
		input.Timezone = "UTC"
		input.Windows = nil
	} else {
		input.Timezone = strings.TrimSpace(input.Timezone)
		if input.Timezone == "" || utf8.RuneCountInString(input.Timezone) > 64 {
			return false
		}
		if _, err := time.LoadLocation(input.Timezone); err != nil {
			return false
		}
		if len(input.Windows) == 0 {
			return false
		}
	}
	for index := range input.Windows {
		window := &input.Windows[index]
		window.Label = strings.TrimSpace(window.Label)
		if window.Label == "" || utf8.RuneCountInString(window.Label) > 64 || window.WeekdayMask < 1 || window.WeekdayMask > 127 || window.StartMinute < 0 || window.StartMinute >= 1440 || window.EndMinute <= window.StartMinute || window.EndMinute > 1440 || !validRates(window.Rates) {
			return false
		}
	}
	for weekday := 0; weekday < 7; weekday++ {
		mask := 1 << weekday
		// 同一窗口可能覆盖多个星期位，因此需要对每个星期独立检查重叠。
		for left := range input.Windows {
			if input.Windows[left].WeekdayMask&mask == 0 {
				continue
			}
			for right := left + 1; right < len(input.Windows); right++ {
				// 两个半开区间相交当且仅当 left.start < right.end 且 right.start < left.end。
				if input.Windows[right].WeekdayMask&mask != 0 && input.Windows[left].StartMinute < input.Windows[right].EndMinute && input.Windows[right].StartMinute < input.Windows[left].EndMinute {
					return false
				}
			}
		}
	}
	return true
}

/**
 * validRates 校验六维费率均为非负且不超过系统上限。
 * @param rates 待校验的六维价格。
 * @return 费率是否有效。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func validRates(rates billing.RateCard) bool {
	for _, value := range []int64{rates.InputMicros, rates.OutputMicros, rates.CacheReadMicros, rates.CacheWriteMicros, rates.CacheWrite1hMicros, rates.RequestMicros} {
		if value < 0 || value > 1_000_000_000_000 {
			return false
		}
	}
	return true
}
