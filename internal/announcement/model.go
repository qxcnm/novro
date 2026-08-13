package announcement

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	SettingKey       = "system_announcement"
	MaximumTitleSize = 120
	MaximumBodySize  = 10_000
)

var ErrInvalidInput = errors.New("invalid system announcement")

// Config is the administrator-managed system announcement.
type Config struct {
	Enabled   bool       `json:"enabled"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

type StoredConfig struct {
	Config
	UpdatedAt time.Time
	Found     bool
}

type Public struct {
	Available bool   `json:"available"`
	Title     string `json:"title"`
	Body      string `json:"body"`
}

/**
 * DefaultConfig 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func DefaultConfig() Config { return Config{} }

/**
 * Normalize 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (c Config) Normalize() Config {
	c.Title = strings.TrimSpace(c.Title)
	c.Body = strings.TrimSpace(c.Body)
	return c
}

/**
 * Validate 用于校验输入或运行状态是否满足要求。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (c Config) Validate() bool {
	c = c.Normalize()
	if utf8.RuneCountInString(c.Title) > MaximumTitleSize || utf8.RuneCountInString(c.Body) > MaximumBodySize {
		return false
	}
	return !c.Enabled || (c.Title != "" && c.Body != "")
}

/**
 * Public 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (c Config) Public() Public {
	c = c.Normalize()
	if !c.Enabled || c.Title == "" || c.Body == "" {
		return Public{}
	}
	return Public{Available: true, Title: c.Title, Body: c.Body}
}
