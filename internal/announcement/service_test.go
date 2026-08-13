package announcement

import (
	"context"
	"errors"
	"testing"
	"time"
)

type memoryStore struct {
	stored StoredConfig
	err    error
}

/**
 * AnnouncementConfig 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *memoryStore) AnnouncementConfig(context.Context) (StoredConfig, error) {
	if s.err != nil {
		return StoredConfig{}, s.err
	}
	return s.stored, nil
}

/**
 * SaveAnnouncementConfig 封装该名称对应的业务处理逻辑。
 * @param config 本次操作使用的配置。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *memoryStore) SaveAnnouncementConfig(_ context.Context, config Config) (StoredConfig, error) {
	if s.err != nil {
		return StoredConfig{}, s.err
	}
	s.stored = StoredConfig{Config: config, UpdatedAt: time.Now(), Found: true}
	return s.stored, nil
}

/**
 * TestConfigReturnsEmptyDefault 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestConfigReturnsEmptyDefault(t *testing.T) {
	got, err := NewService(&memoryStore{}).Config(context.Background())
	if err != nil || got != (Config{}) {
		t.Fatalf("Config() = %+v, err=%v", got, err)
	}
}

/**
 * TestUpdateNormalizesAndPublishes 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestUpdateNormalizesAndPublishes(t *testing.T) {
	store := &memoryStore{}
	got, err := NewService(store).Update(context.Background(), Config{Enabled: true, Title: "  系统公告 ", Body: "  维护通知  "})
	if err != nil || got.Title != "系统公告" || got.Body != "维护通知" || got.UpdatedAt == nil {
		t.Fatalf("Update() = %+v, err=%v", got, err)
	}
	public, err := NewService(store).Public(context.Background())
	if err != nil || !public.Available || public.Title != "系统公告" {
		t.Fatalf("Public() = %+v, err=%v", public, err)
	}
}

/**
 * TestUpdateRejectsIncompleteEnabledAnnouncement 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestUpdateRejectsIncompleteEnabledAnnouncement(t *testing.T) {
	_, err := NewService(&memoryStore{}).Update(context.Background(), Config{Enabled: true, Title: "标题"})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Update() err=%v, want ErrInvalidInput", err)
	}
}

/**
 * TestPublicHidesDisabledDraft 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestPublicHidesDisabledDraft(t *testing.T) {
	store := &memoryStore{stored: StoredConfig{Config: Config{Title: "草稿", Body: "内容"}, Found: true}}
	public, err := NewService(store).Public(context.Background())
	if err != nil || public.Available || public.Title != "" || public.Body != "" {
		t.Fatalf("Public() = %+v, err=%v", public, err)
	}
}
