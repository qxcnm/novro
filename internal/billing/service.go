package billing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Store interface {
	/**
	 * GetSummary 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @param arg3 类型为 EntryFilter 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	GetSummary(context.Context, uuid.UUID, EntryFilter) (Summary, error)
	/**
	 * ListUsage 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @param arg3 类型为 UsageFilter 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	ListUsage(context.Context, uuid.UUID, UsageFilter) (UsagePage, error)
	/**
	 * GetUsageRate 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @param arg3 类型为 time.Time 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	GetUsageRate(context.Context, uuid.UUID, time.Time) (UsageRate, error)
	/**
	 * Adjust 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @param arg3 类型为 uuid.UUID 的接口输入参数。
	 * @param arg4 类型为 uuid.UUID 的接口输入参数。
	 * @param arg5 类型为 int64 的接口输入参数。
	 * @param arg6 类型为 string 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Adjust(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int64, string) (Summary, error)
	/**
	 * Reserve 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @param arg3 类型为 uuid.UUID 的接口输入参数。
	 * @param arg4 类型为 int64 的接口输入参数。
	 * @param arg5 类型为 string 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Reserve(context.Context, uuid.UUID, uuid.UUID, int64, string) error
	/**
	 * Refund 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @param arg3 类型为 uuid.UUID 的接口输入参数。
	 * @param arg4 类型为 int64 的接口输入参数。
	 * @param arg5 类型为 string 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Refund(context.Context, uuid.UUID, uuid.UUID, int64, string) error
	/**
	 * ReleaseReservation 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @param arg3 类型为 uuid.UUID 的接口输入参数。
	 * @param arg4 类型为 int64 的接口输入参数。
	 * @param arg5 类型为 string 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	ReleaseReservation(context.Context, uuid.UUID, uuid.UUID, int64, string) error
	/**
	 * Finalize 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 UsageInput 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Finalize(context.Context, UsageInput) error
	/**
	 * RecordFailure 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 FailureInput 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	RecordFailure(context.Context, FailureInput) error
	/**
	 * StartOperation 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 OperationStartInput 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	StartOperation(context.Context, OperationStartInput) (OperationStartResult, error)
	/**
	 * MarkOperationPendingSettlement 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @param arg3 类型为 UsageInput 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	MarkOperationPendingSettlement(context.Context, uuid.UUID, UsageInput) error
	/**
	 * MarkOperationPendingUnknown 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @param arg3 类型为 string 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	MarkOperationPendingUnknown(context.Context, uuid.UUID, string) error
	/**
	 * CompleteOperation 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	CompleteOperation(context.Context, uuid.UUID) error
	/**
	 * FailOperation 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @param arg3 类型为 string 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	FailOperation(context.Context, uuid.UUID, string) error
	/**
	 * ListPendingSettlements 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 int 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	ListPendingSettlements(context.Context, int) ([]PendingSettlement, error)
	/**
	 * CompensateLegacyUsage 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @param arg3 类型为 uuid.UUID 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	CompensateLegacyUsage(context.Context, uuid.UUID, uuid.UUID) (Summary, int64, error)
}

const usageRateWindow = time.Minute

type Service struct {
	store Store
	now   func() time.Time
}

/**
 * NewService 用于创建并返回所需的对象或记录。
 * @param store 用于持久化和查询数据的存储实现。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func NewService(store Store) *Service { return &Service{store: store, now: time.Now} }

/**
 * Summary 用于计算并返回对应结果。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param userID 目标用户的唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) Summary(ctx context.Context, userID uuid.UUID) (Summary, error) {
	return s.SummaryPage(ctx, userID, EntryFilter{Limit: 20})
}

/**
 * SummaryPage 用于计算并返回对应结果。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param userID 目标用户的唯一标识。
 * @param filter 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) SummaryPage(ctx context.Context, userID uuid.UUID, filter EntryFilter) (Summary, error) {
	if filter.Limit == 0 {
		filter.Limit = 20
	}
	if userID == uuid.Nil || filter.Offset < 0 || filter.Limit < 1 || filter.Limit > 100 {
		return Summary{}, ErrInvalidInput
	}
	return s.store.GetSummary(ctx, userID, filter)
}

/**
 * Usage 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param userID 目标用户的唯一标识。
 * @param filter 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) Usage(ctx context.Context, userID uuid.UUID, filter UsageFilter) (UsagePage, error) {
	filter.Search = strings.TrimSpace(filter.Search)
	filter.Model = strings.TrimSpace(filter.Model)
	if filter.Limit == 0 {
		filter.Limit = 20
	}
	if userID == uuid.Nil || filter.Offset < 0 || filter.Limit < 1 || filter.Limit > 100 || len([]rune(filter.Search)) > 128 || len([]rune(filter.Model)) > 256 ||
		(filter.Status != UsageStatusAll && filter.Status != UsageStatusSuccess && filter.Status != UsageStatusFailed) {
		return UsagePage{}, ErrInvalidInput
	}
	return s.store.ListUsage(ctx, userID, filter)
}

/**
 * UsageRate 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param userID 目标用户的唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) UsageRate(ctx context.Context, userID uuid.UUID) (UsageRate, error) {
	if userID == uuid.Nil {
		return UsageRate{}, ErrInvalidInput
	}
	calculatedAt := s.now().UTC()
	rate, err := s.store.GetUsageRate(ctx, userID, calculatedAt.Add(-usageRateWindow))
	if err != nil {
		return UsageRate{}, err
	}
	rate.WindowSeconds = int(usageRateWindow / time.Second)
	rate.TotalTokens = rate.InputTokens + rate.OutputTokens
	rate.RPM = rate.Requests
	rate.TPM = rate.TotalTokens
	rate.CalculatedAt = calculatedAt
	return rate, nil
}

/**
 * Adjust 用于更新指定的数据或状态。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param userID 目标用户的唯一标识。
 * @param actorID 目标资源的一个或多个唯一标识。
 * @param referenceID 目标资源的一个或多个唯一标识。
 * @param amountMicros 本次操作需要使用的输入参数。
 * @param note 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) Adjust(ctx context.Context, userID, actorID, referenceID uuid.UUID, amountMicros int64, note string) (Summary, error) {
	note = strings.TrimSpace(note)
	if userID == uuid.Nil || actorID == uuid.Nil || referenceID == uuid.Nil || amountMicros == 0 || amountMicros < -1_000_000_000_000_000 || amountMicros > 1_000_000_000_000_000 || note == "" || len([]rune(note)) > 255 {
		return Summary{}, ErrInvalidInput
	}
	return s.store.Adjust(ctx, userID, actorID, referenceID, amountMicros, note)
}

/**
 * Reserve 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param userID 目标用户的唯一标识。
 * @param referenceID 目标资源的一个或多个唯一标识。
 * @param amountMicros 本次操作需要使用的输入参数。
 * @param description 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) Reserve(ctx context.Context, userID, referenceID uuid.UUID, amountMicros int64, description string) error {
	if userID == uuid.Nil || referenceID == uuid.Nil || amountMicros <= 0 || amountMicros > 1_000_000_000_000_000 {
		return ErrInvalidInput
	}
	return s.store.Reserve(ctx, userID, referenceID, amountMicros, description)
}

/**
 * Refund 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param userID 目标用户的唯一标识。
 * @param referenceID 目标资源的一个或多个唯一标识。
 * @param amountMicros 本次操作需要使用的输入参数。
 * @param description 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) Refund(ctx context.Context, userID, referenceID uuid.UUID, amountMicros int64, description string) error {
	if userID == uuid.Nil || referenceID == uuid.Nil || amountMicros <= 0 || amountMicros > 1_000_000_000_000_000 {
		return ErrInvalidInput
	}
	return s.store.Refund(ctx, userID, referenceID, amountMicros, description)
}

/**
 * ReleaseReservation 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param userID 目标用户的唯一标识。
 * @param referenceID 目标资源的一个或多个唯一标识。
 * @param amountMicros 本次操作需要使用的输入参数。
 * @param description 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) ReleaseReservation(ctx context.Context, userID, referenceID uuid.UUID, amountMicros int64, description string) error {
	if userID == uuid.Nil || referenceID == uuid.Nil || amountMicros <= 0 || amountMicros > 1_000_000_000_000_000 {
		return ErrInvalidInput
	}
	return s.store.ReleaseReservation(ctx, userID, referenceID, amountMicros, description)
}

/**
 * Finalize 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param input 需要处理的输入数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) Finalize(ctx context.Context, input UsageInput) error {
	input = normalizeUsageInput(input)
	if err := s.FinalizeInput(input); err != nil {
		return err
	}
	return s.store.Finalize(ctx, input)
}

/**
 * StartOperation 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param input 需要处理的输入数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) StartOperation(ctx context.Context, input OperationStartInput) (OperationStartResult, error) {
	if input.RequestID == uuid.Nil || input.UserID == uuid.Nil || input.APIKeyID == uuid.Nil || len(input.IdempotencyKeyHash) != 64 || len(input.RequestHash) != 64 || input.ReservedMicros < 0 || input.ReservedMicros > 1_000_000_000_000_000 || (input.Endpoint != "chat_completions" && input.Endpoint != "responses" && input.Endpoint != "messages") {
		return OperationStartResult{}, ErrInvalidInput
	}
	return s.store.StartOperation(ctx, input)
}

/**
 * MarkOperationPendingSettlement 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param requestID 目标资源的一个或多个唯一标识。
 * @param input 需要处理的输入数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) MarkOperationPendingSettlement(ctx context.Context, requestID uuid.UUID, input UsageInput) error {
	input = normalizeUsageInput(input)
	if requestID == uuid.Nil || requestID != input.RequestID {
		return ErrInvalidInput
	}
	if err := s.FinalizeInput(input); err != nil {
		return err
	}
	return s.store.MarkOperationPendingSettlement(ctx, requestID, input)
}

/**
 * FinalizeInput 封装该名称对应的业务处理逻辑。
 * @param input 需要处理的输入数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) FinalizeInput(input UsageInput) error {
	input = normalizeUsageInput(input)
	if input.UserID == uuid.Nil || input.APIKeyID == uuid.Nil || input.ModelRouteID == uuid.Nil || input.UpstreamModelID == nil || *input.UpstreamModelID == uuid.Nil || input.BillingGroupID == nil || *input.BillingGroupID == uuid.Nil || input.RequestID == uuid.Nil || input.StatusCode < 200 || input.StatusCode >= 400 || input.InputTokens < 0 || input.OutputTokens < 0 || input.InputTokens != input.Tokens.InputTotal() || input.OutputTokens != input.Tokens.Output || input.CostMicros < 0 || input.BaseCostMicros < 0 || input.CostMicros > 1_000_000_000_000_000 || input.ReservedMicros < 0 || input.ReservedMicros > 1_000_000_000_000_000 || (input.ReservedMicros == 0 && input.CostMicros > 0) || input.CalculationVersion != CalculationVersion || (input.Endpoint != "chat_completions" && input.Endpoint != "responses" && input.Endpoint != "messages") {
		return ErrInvalidInput
	}
	quote, err := CalculateCost(input.Tokens, input.Rates, input.MultiplierBPS)
	if err != nil || quote.BaseCostMicros != input.BaseCostMicros || quote.CostMicros != input.CostMicros {
		return ErrInvalidInput
	}
	return nil
}

/**
 * MarkOperationPendingUnknown 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param requestID 目标资源的一个或多个唯一标识。
 * @param reason 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) MarkOperationPendingUnknown(ctx context.Context, requestID uuid.UUID, reason string) error {
	reason = strings.TrimSpace(reason)
	if requestID == uuid.Nil || reason == "" || len(reason) > 64 {
		return ErrInvalidInput
	}
	return s.store.MarkOperationPendingUnknown(ctx, requestID, reason)
}

/**
 * CompleteOperation 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param requestID 目标资源的一个或多个唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) CompleteOperation(ctx context.Context, requestID uuid.UUID) error {
	if requestID == uuid.Nil {
		return ErrInvalidInput
	}
	return s.store.CompleteOperation(ctx, requestID)
}

/**
 * FailOperation 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param requestID 目标资源的一个或多个唯一标识。
 * @param code 用于标识或筛选目标的文本值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) FailOperation(ctx context.Context, requestID uuid.UUID, code string) error {
	code = strings.TrimSpace(code)
	if requestID == uuid.Nil || code == "" || len(code) > 64 {
		return ErrInvalidInput
	}
	return s.store.FailOperation(ctx, requestID, code)
}

/**
 * ListPendingSettlements 用于筛选并返回数据列表。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param limit 本次操作使用的数值参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) ListPendingSettlements(ctx context.Context, limit int) ([]PendingSettlement, error) {
	if limit < 1 || limit > 100 {
		return nil, ErrInvalidInput
	}
	return s.store.ListPendingSettlements(ctx, limit)
}

/**
 * RecoverPendingSettlements 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param limit 本次操作使用的数值参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) RecoverPendingSettlements(ctx context.Context, limit int) (int, error) {
	pending, err := s.ListPendingSettlements(ctx, limit)
	if err != nil {
		return 0, err
	}
	recovered := 0
	var recoveryErrors []error
	for _, item := range pending {
		usage := normalizeUsageInput(item.Usage)
		if err := s.FinalizeInput(usage); err != nil {
			recoveryErrors = append(recoveryErrors, fmt.Errorf("validate pending settlement %s: %w", item.Operation.RequestID, err))
			continue
		}
		if err := s.store.Finalize(ctx, usage); err != nil {
			recoveryErrors = append(recoveryErrors, fmt.Errorf("finalize pending settlement %s: %w", item.Operation.RequestID, err))
			continue
		}
		if err := s.store.CompleteOperation(ctx, item.Operation.RequestID); err != nil {
			recoveryErrors = append(recoveryErrors, fmt.Errorf("complete pending settlement %s: %w", item.Operation.RequestID, err))
			continue
		}
		recovered++
	}
	return recovered, errors.Join(recoveryErrors...)
}

/**
 * normalizeUsageInput 封装该名称对应的业务处理逻辑。
 * @param input 需要处理的输入数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func normalizeUsageInput(input UsageInput) UsageInput {
	if input.StatusCode == 0 {
		input.StatusCode = 200
	}
	return input
}

/**
 * CompensateLegacyUsage 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param requestID 目标资源的一个或多个唯一标识。
 * @param actorID 目标资源的一个或多个唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) CompensateLegacyUsage(ctx context.Context, requestID, actorID uuid.UUID) (Summary, int64, error) {
	if requestID == uuid.Nil || actorID == uuid.Nil {
		return Summary{}, 0, ErrInvalidInput
	}
	return s.store.CompensateLegacyUsage(ctx, requestID, actorID)
}

/**
 * RecordFailure 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param input 需要处理的输入数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) RecordFailure(ctx context.Context, input FailureInput) error {
	input.ErrorCode = strings.TrimSpace(input.ErrorCode)
	input.ErrorMessage = strings.TrimSpace(input.ErrorMessage)
	if input.StatusCode == 0 {
		input.StatusCode = 502
	}
	if input.UserID == uuid.Nil || input.APIKeyID == uuid.Nil || input.ModelRouteID == uuid.Nil || input.UpstreamModelID == nil || *input.UpstreamModelID == uuid.Nil || input.BillingGroupID == nil || *input.BillingGroupID == uuid.Nil || input.RequestID == uuid.Nil || (input.Endpoint != "chat_completions" && input.Endpoint != "responses" && input.Endpoint != "messages") || input.StatusCode < 400 || input.StatusCode > 599 || input.ErrorCode == "" || len(input.ErrorCode) > 64 || input.ErrorMessage == "" || len([]rune(input.ErrorMessage)) > 1024 || input.DurationMS < 0 || input.DurationMS > 86_400_000 || input.MultiplierBPS <= 0 || input.MultiplierBPS > 1_000_000 || input.ModelName == "" || len([]rune(input.ModelName)) > 128 || len([]rune(input.UpstreamModelName)) > 256 || len([]rune(input.BillingGroupCode)) > 64 || len([]rune(input.BillingGroupName)) > 128 {
		return ErrInvalidInput
	}
	return s.store.RecordFailure(ctx, input)
}
