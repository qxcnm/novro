package modelpricing

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/internal/billing"
)

/**
 * Mode 表示价格方案的计费模式。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
type Mode string

const (
	ModeFixed     Mode = "fixed"
	ModeScheduled Mode = "scheduled"
)

/**
 * Status 表示价格方案的生命周期状态。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
	StatusRetired   Status = "retired"
)

var (
	ErrInvalidInput = errors.New("invalid model pricing input")
	ErrNotFound     = errors.New("model price plan not found")
	ErrModelMissing = errors.New("upstream model not found")
	ErrImmutable    = errors.New("published model price plan is immutable")
	ErrConflict     = errors.New("model price plan conflicts with an existing version")
	ErrNoPrice      = errors.New("no effective model price plan")
)

/**
 * Window 表示按星期和本地分钟生效的峰谷价格窗口。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
type Window struct {
	ID          uuid.UUID        `json:"id"`
	Label       string           `json:"label"`
	WeekdayMask int              `json:"weekday_mask"`
	StartMinute int              `json:"start_minute"`
	EndMinute   int              `json:"end_minute"`
	Rates       billing.RateCard `json:"rates"`
	CreatedAt   time.Time        `json:"created_at"`
}

/**
 * Plan 表示一个版本化、可发布的模型价格方案及其窗口。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
type Plan struct {
	ID              uuid.UUID        `json:"id"`
	UpstreamModelID uuid.UUID        `json:"upstream_model_id"`
	Version         int              `json:"version"`
	Mode            Mode             `json:"mode"`
	Timezone        string           `json:"timezone"`
	EffectiveFrom   time.Time        `json:"effective_from"`
	EffectiveTo     *time.Time       `json:"effective_to,omitempty"`
	Status          Status           `json:"status"`
	DefaultRates    billing.RateCard `json:"default_rates"`
	Windows         []Window         `json:"windows"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

/**
 * RepublishResult 表示历史价格版本切换后的结果。
 * Created 字段为兼容既有 API 响应而保留；当前切换逻辑始终直接调整已有版本，因此固定为 false。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
type RepublishResult struct {
	Plan    Plan
	Created bool
}

/**
 * WindowInput 表示创建或更新价格方案时提交的窗口配置。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
type WindowInput struct {
	Label       string           `json:"label"`
	WeekdayMask int              `json:"weekday_mask"`
	StartMinute int              `json:"start_minute"`
	EndMinute   int              `json:"end_minute"`
	Rates       billing.RateCard `json:"rates"`
}

/**
 * PlanInput 表示创建或更新价格方案时提交的完整配置。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
type PlanInput struct {
	Mode          Mode             `json:"mode"`
	Timezone      string           `json:"timezone"`
	EffectiveFrom time.Time        `json:"effective_from"`
	EffectiveTo   *time.Time       `json:"effective_to,omitempty"`
	DefaultRates  billing.RateCard `json:"default_rates"`
	Windows       []WindowInput    `json:"windows"`
}

/**
 * Resolution 表示请求开始时解析出的固定费率和价格来源。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
type Resolution struct {
	Rates          billing.RateCard `json:"rates"`
	PlanID         *uuid.UUID       `json:"plan_id,omitempty"`
	PlanVersion    int              `json:"plan_version"`
	WindowID       *uuid.UUID       `json:"window_id,omitempty"`
	WindowLabel    string           `json:"window_label"`
	Timezone       string           `json:"timezone"`
	EffectiveFrom  time.Time        `json:"effective_from"`
	LegacyFallback bool             `json:"legacy_fallback"`
}
