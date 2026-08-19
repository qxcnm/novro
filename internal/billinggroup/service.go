package billinggroup

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var codePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$`)

type Store interface {

	/**
	 * Create 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 CreateInput 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Create(context.Context, CreateInput) (Record, error)

	/**
	 * List 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 ListFilter 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	List(context.Context, ListFilter) ([]Record, error)

	/**
	 * Update 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @param arg3 类型为 UpdateInput 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Update(context.Context, uuid.UUID, UpdateInput) (Record, error)

	/**
	 * SetStatus 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @param arg3 类型为 Status 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	SetStatus(context.Context, uuid.UUID, Status) (Record, error)

	/**
	 * Delete 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Delete(context.Context, uuid.UUID) error
}

type Service struct {
	store Store

	cacheMu sync.RWMutex
	cache   map[uuid.UUID]Summary
	removed map[uuid.UUID]struct{}
}

/**
 * NewService 执行该名称对应的业务处理逻辑。
 * @param store 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func NewService(store Store) *Service {
	return &Service{store: store, cache: make(map[uuid.UUID]Summary), removed: make(map[uuid.UUID]struct{})}
}

// MultiplierAt resolves a billing-group multiplier from the process-wide
// snapshot. Authentication already provides a database-backed fallback on the
// first request; later requests share the cached discount without another read.
func (s *Service) MultiplierAt(fallback Summary, at time.Time) int64 {
	if fallback.ID == uuid.Nil {
		return fallback.MultiplierAt(at)
	}
	s.cacheMu.RLock()
	cached, ok := s.cache[fallback.ID]
	_, removed := s.removed[fallback.ID]
	s.cacheMu.RUnlock()
	if removed {
		return fallback.MultiplierAt(at)
	}
	if !ok {
		s.cacheMu.Lock()
		cached, ok = s.cache[fallback.ID]
		_, removed = s.removed[fallback.ID]
		if !ok && !removed {
			cached = cloneSummary(fallback)
			s.cache[fallback.ID] = cached
		}
		s.cacheMu.Unlock()
		if removed {
			return fallback.MultiplierAt(at)
		}
	}
	return cached.MultiplierAt(at)
}

/**
 * Create 执行该名称对应的业务处理逻辑。
 * @param ctx 本次操作需要使用的输入参数。
 * @param input 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) Create(ctx context.Context, input CreateInput) (Record, error) {
	input.Code = strings.ToLower(strings.TrimSpace(input.Code))
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if input.Kind == "" {
		input.Kind = KindStandard
	}
	if input.Kind == KindComposite && input.MultiplierBPS == 0 {
		input.MultiplierBPS = DefaultMultiplierBPS
	}
	if !codePattern.MatchString(input.Code) || !validName(input.DisplayName) || !validKind(input.Kind) || !validMultiplier(input.MultiplierBPS) || !validAuthorizedUserIDs(input.AuthorizedUserIDs) || !validMemberGroupIDs(input.MemberGroupIDs) || (!input.IsHidden && len(input.AuthorizedUserIDs) > 0) || !normalizeDiscount(input.Discount) {
		return Record{}, ErrInvalidInput
	}
	if input.Kind == KindComposite {
		if input.MultiplierBPS != DefaultMultiplierBPS || input.Discount != nil || len(input.MemberGroupIDs) == 0 {
			return Record{}, ErrInvalidInput
		}
	} else if len(input.MemberGroupIDs) > 0 {
		return Record{}, ErrInvalidInput
	}
	record, err := s.store.Create(ctx, input)
	if err == nil {
		record = normalizeRecord(record)
		s.remember(record)
	}
	return record, err
}

/**
 * List 执行该名称对应的业务处理逻辑。
 * @param ctx 本次操作需要使用的输入参数。
 * @param filter 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) List(ctx context.Context, filter ListFilter) ([]Record, error) {
	filter.Search = strings.TrimSpace(filter.Search)
	if filter.Status != "" && filter.Status != StatusActive && filter.Status != StatusDisabled {
		return nil, ErrInvalidInput
	}
	records, err := s.store.List(ctx, filter)
	if err == nil {
		for index, record := range records {
			records[index] = normalizeRecord(record)
			s.remember(record)
		}
	}
	return records, err
}

/**
 * Update 执行该名称对应的业务处理逻辑。
 * @param ctx 本次操作需要使用的输入参数。
 * @param id 本次操作需要使用的输入参数。
 * @param input 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) Update(ctx context.Context, id uuid.UUID, input UpdateInput) (Record, error) {
	if id == uuid.Nil || (input.DisplayName == nil && input.Kind == nil && input.MultiplierBPS == nil && input.IsHidden == nil && input.AuthorizedUserIDs == nil && input.MemberGroupIDs == nil && input.Discount == nil && !input.ClearDiscount) || (input.Discount != nil && input.ClearDiscount) {
		return Record{}, ErrInvalidInput
	}
	if input.Kind != nil && !validKind(*input.Kind) {
		return Record{}, ErrInvalidInput
	}
	if input.DisplayName != nil {
		value := strings.TrimSpace(*input.DisplayName)
		if !validName(value) {
			return Record{}, ErrInvalidInput
		}
		input.DisplayName = &value
	}
	if input.MultiplierBPS != nil && !validMultiplier(*input.MultiplierBPS) {
		return Record{}, ErrInvalidInput
	}
	if input.AuthorizedUserIDs != nil && !validAuthorizedUserIDs(*input.AuthorizedUserIDs) {
		return Record{}, ErrInvalidInput
	}
	if input.MemberGroupIDs != nil && !validMemberGroupIDs(*input.MemberGroupIDs) {
		return Record{}, ErrInvalidInput
	}
	if !normalizeDiscount(input.Discount) {
		return Record{}, ErrInvalidInput
	}
	record, err := s.store.Update(ctx, id, input)
	if err == nil {
		record = normalizeRecord(record)
		s.remember(record)
	}
	return record, err
}

/**
 * SetStatus 执行该名称对应的业务处理逻辑。
 * @param ctx 本次操作需要使用的输入参数。
 * @param id 本次操作需要使用的输入参数。
 * @param status 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) SetStatus(ctx context.Context, id uuid.UUID, status Status) (Record, error) {
	if id == uuid.Nil || (status != StatusActive && status != StatusDisabled) {
		return Record{}, ErrInvalidInput
	}
	record, err := s.store.SetStatus(ctx, id, status)
	if err == nil {
		record = normalizeRecord(record)
		s.remember(record)
	}
	return record, err
}

/**
 * Delete 执行该名称对应的业务处理逻辑。
 * @param ctx 本次操作需要使用的输入参数。
 * @param id 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return ErrInvalidInput
	}
	if err := s.store.Delete(ctx, id); err != nil {
		return err
	}
	s.cacheMu.Lock()
	delete(s.cache, id)
	s.removed[id] = struct{}{}
	s.cacheMu.Unlock()
	return nil
}

func (s *Service) remember(record Record) {
	if record.ID == uuid.Nil {
		return
	}
	summary := Summary{
		ID: record.ID, Code: record.Code, DisplayName: record.DisplayName, Kind: record.Kind, MultiplierBPS: record.MultiplierBPS,
		DiscountName: record.DiscountName, DiscountMultiplierBPS: record.DiscountMultiplierBPS,
		DiscountStartsAt: record.DiscountStartsAt, DiscountEndsAt: record.DiscountEndsAt,
	}
	for _, member := range record.MemberGroups {
		summary.MemberGroupIDs = append(summary.MemberGroupIDs, member.ID)
	}
	summary.MemberGroupCount = len(summary.MemberGroupIDs)
	s.cacheMu.Lock()
	// A list or update may have read this record just before a concurrent delete
	// committed. Keep the delete tombstone authoritative so that stale results
	// cannot restore a removed discount snapshot.
	if _, removed := s.removed[record.ID]; removed {
		s.cacheMu.Unlock()
		return
	}
	s.cache[record.ID] = cloneSummary(summary)
	s.cacheMu.Unlock()
}

func cloneSummary(summary Summary) Summary {
	if summary.DiscountStartsAt != nil {
		value := *summary.DiscountStartsAt
		summary.DiscountStartsAt = &value
	}
	if summary.DiscountEndsAt != nil {
		value := *summary.DiscountEndsAt
		summary.DiscountEndsAt = &value
	}
	return summary
}

/**
 * validName 执行该名称对应的业务处理逻辑。
 * @param value 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func validName(value string) bool { return value != "" && utf8.RuneCountInString(value) <= 128 }

/**
 * validMultiplier 执行该名称对应的业务处理逻辑。
 * @param value 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func validMultiplier(value int64) bool { return value >= 1 && value <= 1_000_000 }

func validKind(kind Kind) bool { return kind == KindStandard || kind == KindComposite }

func normalizeDiscount(discount *DiscountInput) bool {
	if discount == nil {
		return true
	}
	discount.Name = strings.TrimSpace(discount.Name)
	if discount.Name == "" || utf8.RuneCountInString(discount.Name) > 64 || discount.MultiplierBPS < 1 || discount.MultiplierBPS >= DefaultMultiplierBPS || discount.StartsAt.IsZero() || discount.EndsAt.IsZero() || !discount.EndsAt.After(discount.StartsAt) {
		return false
	}
	discount.StartsAt = discount.StartsAt.UTC()
	discount.EndsAt = discount.EndsAt.UTC()
	return true
}

/**
 * validAuthorizedUserIDs 执行该名称对应的业务处理逻辑。
 * @param ids 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func validAuthorizedUserIDs(ids []uuid.UUID) bool {
	seen := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			return false
		}
		if _, exists := seen[id]; exists {
			return false
		}
		seen[id] = struct{}{}
	}
	return true
}

func validMemberGroupIDs(ids []uuid.UUID) bool {
	return validAuthorizedUserIDs(ids)
}
