package gatewaysettings

import (
	"errors"
	"time"
)

const (
	SettingKey = "gateway_request_settings"

	DefaultSSEHeartbeatIntervalMS      = 15_000
	DefaultUpstreamTimeoutMS           = 0
	DefaultUpstreamStreamIdleTimeoutMS = 0
	DefaultReservationInputTokenCap    = 16_384
	DefaultReservationOutputTokenCap   = 1024
	minimumConfiguredTimeoutMS         = 1_000
	maximumConfiguredTimeoutMS         = 24 * 60 * 60 * 1_000
	maximumHeartbeatIntervalMS         = 60 * 60 * 1_000
	maximumReservationInputTokenCap    = 1_000_000
	maximumReservationOutputTokenCap   = 1_000_000
)

var ErrInvalidConfig = errors.New("invalid gateway request settings")

// Config controls the request lifecycle for all model gateway endpoints.
type Config struct {
	SSEHeartbeatEnabled         bool       `json:"sse_heartbeat_enabled"`
	SSEHeartbeatIntervalMS      int        `json:"sse_heartbeat_interval_ms"`
	UpstreamTimeoutMS           int        `json:"upstream_timeout_ms"`
	UpstreamStreamIdleTimeoutMS int        `json:"upstream_stream_idle_timeout_ms"`
	ReservationInputTokenCap    int        `json:"reservation_input_token_cap"`
	ReservationOutputTokenCap   int        `json:"reservation_output_token_cap"`
	UpdatedAt                   *time.Time `json:"updated_at,omitempty"`
}

type StoredConfig struct {
	Config
	UpdatedAt time.Time
	Found     bool
}

/**
 * DefaultConfig 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func DefaultConfig() Config {
	return Config{
		SSEHeartbeatEnabled:         true,
		SSEHeartbeatIntervalMS:      DefaultSSEHeartbeatIntervalMS,
		UpstreamTimeoutMS:           DefaultUpstreamTimeoutMS,
		UpstreamStreamIdleTimeoutMS: DefaultUpstreamStreamIdleTimeoutMS,
		ReservationInputTokenCap:    DefaultReservationInputTokenCap,
		ReservationOutputTokenCap:   DefaultReservationOutputTokenCap,
	}
}

/**
 * Validate 用于校验输入或运行状态是否满足要求。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (c Config) Validate() bool {
	if c.SSEHeartbeatIntervalMS < minimumConfiguredTimeoutMS || c.SSEHeartbeatIntervalMS > maximumHeartbeatIntervalMS {
		return false
	}
	if !validOptionalTimeout(c.UpstreamTimeoutMS) || !validOptionalTimeout(c.UpstreamStreamIdleTimeoutMS) {
		return false
	}
	if c.ReservationInputTokenCap < 1 || c.ReservationInputTokenCap > maximumReservationInputTokenCap ||
		c.ReservationOutputTokenCap < 1 || c.ReservationOutputTokenCap > maximumReservationOutputTokenCap {
		return false
	}
	return true
}

/**
 * withDefaults 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (c Config) withDefaults() Config {
	if c.ReservationInputTokenCap == 0 {
		c.ReservationInputTokenCap = DefaultReservationInputTokenCap
	}
	if c.ReservationOutputTokenCap == 0 {
		c.ReservationOutputTokenCap = DefaultReservationOutputTokenCap
	}
	return c
}

/**
 * validOptionalTimeout 封装该名称对应的业务处理逻辑。
 * @param value 需要处理的输入值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func validOptionalTimeout(value int) bool {
	return value == 0 || (value >= minimumConfiguredTimeoutMS && value <= maximumConfiguredTimeoutMS)
}

/**
 * HeartbeatInterval 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (c Config) HeartbeatInterval() time.Duration {
	return time.Duration(c.SSEHeartbeatIntervalMS) * time.Millisecond
}

/**
 * UpstreamTimeout 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (c Config) UpstreamTimeout() time.Duration {
	return time.Duration(c.UpstreamTimeoutMS) * time.Millisecond
}

/**
 * UpstreamStreamIdleTimeout 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (c Config) UpstreamStreamIdleTimeout() time.Duration {
	return time.Duration(c.UpstreamStreamIdleTimeoutMS) * time.Millisecond
}
