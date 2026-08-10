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
	minimumConfiguredTimeoutMS         = 1_000
	maximumConfiguredTimeoutMS         = 24 * 60 * 60 * 1_000
	maximumHeartbeatIntervalMS         = 60 * 60 * 1_000
)

var ErrInvalidConfig = errors.New("invalid gateway request settings")

// Config controls the request lifecycle for all model gateway endpoints.
type Config struct {
	SSEHeartbeatEnabled         bool       `json:"sse_heartbeat_enabled"`
	SSEHeartbeatIntervalMS      int        `json:"sse_heartbeat_interval_ms"`
	UpstreamTimeoutMS           int        `json:"upstream_timeout_ms"`
	UpstreamStreamIdleTimeoutMS int        `json:"upstream_stream_idle_timeout_ms"`
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
	}
}

func (c Config) Validate() bool {
	if c.SSEHeartbeatIntervalMS < minimumConfiguredTimeoutMS || c.SSEHeartbeatIntervalMS > maximumHeartbeatIntervalMS {
		return false
	}
	if !validOptionalTimeout(c.UpstreamTimeoutMS) || !validOptionalTimeout(c.UpstreamStreamIdleTimeoutMS) {
		return false
	}
	return true
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
