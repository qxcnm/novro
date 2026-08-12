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

func (c Config) withDefaults() Config {
	if c.ReservationInputTokenCap == 0 {
		c.ReservationInputTokenCap = DefaultReservationInputTokenCap
	}
	if c.ReservationOutputTokenCap == 0 {
		c.ReservationOutputTokenCap = DefaultReservationOutputTokenCap
	}
	return c
}

func validOptionalTimeout(value int) bool {
	return value == 0 || (value >= minimumConfiguredTimeoutMS && value <= maximumConfiguredTimeoutMS)
}

func (c Config) HeartbeatInterval() time.Duration {
	return time.Duration(c.SSEHeartbeatIntervalMS) * time.Millisecond
}

func (c Config) UpstreamTimeout() time.Duration {
	return time.Duration(c.UpstreamTimeoutMS) * time.Millisecond
}

func (c Config) UpstreamStreamIdleTimeout() time.Duration {
	return time.Duration(c.UpstreamStreamIdleTimeoutMS) * time.Millisecond
}
