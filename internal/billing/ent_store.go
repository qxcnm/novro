package billing

import (
	"context"
	stdsql "database/sql"
	"fmt"
	"math"
	"sort"
	"time"
	"unicode/utf8"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	"github.com/novro-gateway/novro/ent"
	entapiusage "github.com/novro-gateway/novro/ent/apiusage"
	"github.com/novro-gateway/novro/ent/predicate"
	entwallet "github.com/novro-gateway/novro/ent/wallet"
	entwalletentry "github.com/novro-gateway/novro/ent/walletentry"
)

type EntStore struct{ client *ent.Client }

func NewEntStore(client *ent.Client) *EntStore { return &EntStore{client: client} }

func (s *EntStore) GetSummary(ctx context.Context, userID uuid.UUID, filter EntryFilter) (Summary, error) {
	walletEntity, err := s.client.Wallet.Query().Where(entwallet.UserIDEQ(userID)).Only(ctx)
	if ent.IsNotFound(err) {
		return Summary{}, ErrWalletNotFound
	}
	if err != nil {
		return Summary{}, fmt.Errorf("read wallet: %w", err)
	}
	entriesQuery := walletEntity.QueryEntries()
	total, err := entriesQuery.Clone().Count(ctx)
	if err != nil {
		return Summary{}, fmt.Errorf("count wallet entries: %w", err)
	}
	entries, err := entriesQuery.Clone().Order(ent.Desc(entwalletentry.FieldCreatedAt), ent.Desc(entwalletentry.FieldID)).Offset(filter.Offset).Limit(filter.Limit).All(ctx)
	if err != nil {
		return Summary{}, fmt.Errorf("list wallet entries: %w", err)
	}
	var activeReservations []struct {
		Amount stdsql.NullInt64 `json:"amount_micros"`
	}
	reservationQuery := walletEntity.QueryEntries().Where(
		entwalletentry.EntryTypeEQ(entwalletentry.EntryTypeUsageReservation),
		func(selector *entsql.Selector) {
			usage := entsql.Table(entapiusage.Table).As("reserved_usage")
			selector.Where(entsql.Not(entsql.Exists(
				entsql.Select(usage.C(entapiusage.FieldRequestID)).From(usage).Where(entsql.And(
					entsql.ColumnsEQ(usage.C(entapiusage.FieldRequestID), selector.C(entwalletentry.FieldReferenceID)),
					entsql.LT(usage.C(entapiusage.FieldStatusCode), 400),
				)),
			)))
		},
		func(selector *entsql.Selector) {
			refund := entsql.Table(entwalletentry.Table).As("reservation_refund")
			selector.Where(entsql.Not(entsql.Exists(
				entsql.Select(refund.C(entwalletentry.FieldReferenceID)).From(refund).Where(entsql.And(
					entsql.ColumnsEQ(refund.C(entwalletentry.FieldWalletID), selector.C(entwalletentry.FieldWalletID)),
					entsql.ColumnsEQ(refund.C(entwalletentry.FieldReferenceID), selector.C(entwalletentry.FieldReferenceID)),
					entsql.EQ(refund.C(entwalletentry.FieldEntryType), entwalletentry.EntryTypeUsageRefund),
				)),
			)))
		},
	)
	if err := reservationQuery.Aggregate(ent.As(ent.Sum(entwalletentry.FieldAmountMicros), "amount_micros")).Scan(ctx, &activeReservations); err != nil {
		return Summary{}, fmt.Errorf("summarize active usage reservations: %w", err)
	}
	var reservedMicros int64
	if len(activeReservations) > 0 {
		reservedMicros = -activeReservations[0].Amount.Int64
	}
	return Summary{
		Wallet: walletFromEnt(walletEntity), Entries: entriesFromEnt(entries), EntriesTotal: total,
		EntriesOffset: filter.Offset, EntriesLimit: filter.Limit, ReservedMicros: reservedMicros,
	}, nil
}

func (s *EntStore) ListUsage(ctx context.Context, userID uuid.UUID, filter UsageFilter) (UsagePage, error) {
	query := s.client.APIUsage.Query().Where(entapiusage.UserIDEQ(userID))
	if filter.APIKeyID != uuid.Nil {
		query.Where(entapiusage.APIKeyIDEQ(filter.APIKeyID))
	}
	if filter.Model != "" {
		query.Where(entapiusage.ModelNameEQ(filter.Model))
	}
	switch filter.Status {
	case UsageStatusSuccess:
		query.Where(entapiusage.StatusCodeLT(400))
	case UsageStatusFailed:
		query.Where(entapiusage.StatusCodeGTE(400))
	}
	if filter.From != nil {
		query.Where(entapiusage.CreatedAtGTE(*filter.From))
	}
	if filter.Search != "" {
		predicates := []predicate.APIUsage{
			entapiusage.ModelNameContainsFold(filter.Search),
			entapiusage.UpstreamModelNameContainsFold(filter.Search),
			entapiusage.UpstreamRequestIDContainsFold(filter.Search),
		}
		if requestID, err := uuid.Parse(filter.Search); err == nil {
			predicates = append(predicates, entapiusage.RequestIDEQ(requestID))
		}
		query.Where(entapiusage.Or(predicates...))
	}
	total, err := query.Clone().Count(ctx)
	if err != nil {
		return UsagePage{}, fmt.Errorf("count API usage: %w", err)
	}
	var aggregates []struct {
		InputTokens  stdsql.NullInt64 `json:"input_tokens"`
		OutputTokens stdsql.NullInt64 `json:"output_tokens"`
		CostMicros   stdsql.NullInt64 `json:"cost_micros"`
	}
	if err := query.Clone().Aggregate(
		ent.As(ent.Sum(entapiusage.FieldInputTokens), "input_tokens"),
		ent.As(ent.Sum(entapiusage.FieldOutputTokens), "output_tokens"),
		ent.As(ent.Sum(entapiusage.FieldCostMicros), "cost_micros"),
	).Scan(ctx, &aggregates); err != nil {
		return UsagePage{}, fmt.Errorf("summarize API usage: %w", err)
	}
	entities, err := query.Clone().WithModelRoute().WithAPIKey().Order(ent.Desc(entapiusage.FieldCreatedAt), ent.Desc(entapiusage.FieldID)).Offset(filter.Offset).Limit(filter.Limit).All(ctx)
	if err != nil {
		return UsagePage{}, fmt.Errorf("list API usage: %w", err)
	}
	items := make([]Usage, 0, len(entities))
	for _, entity := range entities {
		model, _ := entity.Edges.ModelRouteOrErr()
		modelName := ""
		if model != nil {
			modelName = model.PublicName
		}
		apiKey, _ := entity.Edges.APIKeyOrErr()
		apiKeyName := ""
		if apiKey != nil {
			apiKeyName = apiKey.Name
		}
		if entity.ModelName != "" {
			modelName = entity.ModelName
		}
		items = append(items, Usage{ID: entity.ID, RequestID: entity.RequestID, APIKeyID: entity.APIKeyID, APIKeyName: apiKeyName, ModelName: modelName, Endpoint: string(entity.Endpoint), StatusCode: entity.StatusCode, ErrorCode: entity.ErrorCode, ErrorMessage: entity.ErrorMessage, InputTokens: entity.InputTokens,
			UncachedInputTokens: entity.UncachedInputTokens, CacheReadInputTokens: entity.CacheReadInputTokens, CacheWriteInputTokens: entity.CacheWriteInputTokens, CacheWrite1hInputTokens: entity.CacheWrite1hInputTokens, OutputTokens: entity.OutputTokens,
			Rates:          RateCard{InputMicros: entity.InputPriceMicros, OutputMicros: entity.OutputPriceMicros, CacheReadMicros: entity.CacheReadPriceMicros, CacheWriteMicros: entity.CacheWritePriceMicros, CacheWrite1hMicros: entity.CacheWrite1hPriceMicros, RequestMicros: entity.RequestPriceMicros},
			BaseCostMicros: entity.BaseCostMicros, MultiplierBPS: entity.MultiplierBps, CostMicros: entity.CostMicros, ReservedMicros: entity.ReservedMicros, BillingGroupCode: entity.BillingGroupCode, BillingGroupName: entity.BillingGroupName, UpstreamModelName: entity.UpstreamModelName, CalculationVersion: entity.CalculationVersion,
			Estimated: entity.Estimated, UpstreamRequestID: entity.UpstreamRequestID, CreatedAt: entity.CreatedAt, FinishedAt: entity.FinishedAt, DurationMS: entity.DurationMs})
	}
	var modelRows []struct {
		Model string `json:"model_name"`
	}
	modelQuery := s.client.APIUsage.Query().Where(entapiusage.UserIDEQ(userID), entapiusage.ModelNameNEQ(""))
	if err := modelQuery.GroupBy(entapiusage.FieldModelName).Scan(ctx, &modelRows); err != nil {
		return UsagePage{}, fmt.Errorf("list API usage models: %w", err)
	}
	models := make([]string, 0, len(modelRows))
	for _, row := range modelRows {
		models = append(models, row.Model)
	}
	sort.Strings(models)
	page := UsagePage{Usage: items, Models: models, Total: total, Offset: filter.Offset, Limit: filter.Limit}
	if len(aggregates) > 0 {
		page.TotalTokens = aggregates[0].InputTokens.Int64 + aggregates[0].OutputTokens.Int64
		page.TotalCostMicros = aggregates[0].CostMicros.Int64
	}
	return page, nil
}

func (s *EntStore) Adjust(ctx context.Context, userID, actorID uuid.UUID, amount int64, note string) (Summary, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return Summary{}, fmt.Errorf("begin balance adjustment: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	walletEntity, err := tx.Wallet.Query().Where(entwallet.UserIDEQ(userID)).ForUpdate().Only(ctx)
	if ent.IsNotFound(err) {
		return Summary{}, ErrWalletNotFound
	}
	if err != nil {
		return Summary{}, fmt.Errorf("lock wallet: %w", err)
	}
	if (amount < 0 && walletEntity.BalanceMicros < -amount) || (amount > 0 && walletEntity.BalanceMicros > math.MaxInt64-amount) {
		return Summary{}, ErrInsufficientBalance
	}
	next := walletEntity.BalanceMicros + amount
	updated, err := tx.Wallet.UpdateOneID(walletEntity.ID).SetBalanceMicros(next).Save(ctx)
	if err != nil {
		return Summary{}, fmt.Errorf("update wallet balance: %w", err)
	}
	if _, err := tx.WalletEntry.Create().SetWalletID(walletEntity.ID).SetActorUserID(actorID).SetReferenceID(uuid.New()).SetEntryType(entwalletentry.EntryTypeManualAdjustment).SetAmountMicros(amount).SetBalanceAfterMicros(next).SetDescription(note).Save(ctx); err != nil {
		return Summary{}, fmt.Errorf("record balance adjustment: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Summary{}, fmt.Errorf("commit balance adjustment: %w", err)
	}
	return s.GetSummary(ctx, updated.UserID, EntryFilter{Limit: 20})
}

func (s *EntStore) Reserve(ctx context.Context, userID, referenceID uuid.UUID, amount int64, description string) error {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin usage reservation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	walletEntity, err := tx.Wallet.Query().Where(entwallet.UserIDEQ(userID)).ForUpdate().Only(ctx)
	if ent.IsNotFound(err) {
		return ErrWalletNotFound
	}
	if err != nil {
		return fmt.Errorf("lock wallet for usage: %w", err)
	}
	existing, err := tx.WalletEntry.Query().Where(
		entwalletentry.WalletIDEQ(walletEntity.ID),
		entwalletentry.ReferenceIDEQ(referenceID),
		entwalletentry.EntryTypeEQ(entwalletentry.EntryTypeUsageReservation),
	).Only(ctx)
	if err == nil {
		if existing.AmountMicros == -amount {
			return nil
		}
		return ErrRequestConflict
	}
	if !ent.IsNotFound(err) {
		return fmt.Errorf("read existing usage reservation: %w", err)
	}
	if walletEntity.BalanceMicros < amount {
		return ErrInsufficientBalance
	}
	next := walletEntity.BalanceMicros - amount
	if _, err := tx.Wallet.UpdateOneID(walletEntity.ID).SetBalanceMicros(next).Save(ctx); err != nil {
		return fmt.Errorf("reserve usage balance: %w", err)
	}
	if _, err := tx.WalletEntry.Create().SetWalletID(walletEntity.ID).SetReferenceID(referenceID).SetEntryType(entwalletentry.EntryTypeUsageReservation).SetAmountMicros(-amount).SetBalanceAfterMicros(next).SetDescription(limitDescription(description)).Save(ctx); err != nil {
		return fmt.Errorf("record usage reservation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit usage reservation: %w", err)
	}
	return nil
}

func (s *EntStore) Refund(ctx context.Context, userID, referenceID uuid.UUID, amount int64, description string) error {
	return s.releaseReservation(ctx, userID, referenceID, amount, description, false)
}

func (s *EntStore) ReleaseReservation(ctx context.Context, userID, referenceID uuid.UUID, amount int64, description string) error {
	return s.releaseReservation(ctx, userID, referenceID, amount, description, true)
}

func (s *EntStore) releaseReservation(ctx context.Context, userID, referenceID uuid.UUID, amount int64, description string, requireUnfinalized bool) error {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin usage refund: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	walletEntity, err := tx.Wallet.Query().Where(entwallet.UserIDEQ(userID)).ForUpdate().Only(ctx)
	if ent.IsNotFound(err) {
		return ErrWalletNotFound
	}
	if err != nil {
		return fmt.Errorf("lock wallet for refund: %w", err)
	}
	reservation, err := tx.WalletEntry.Query().Where(
		entwalletentry.WalletIDEQ(walletEntity.ID),
		entwalletentry.ReferenceIDEQ(referenceID),
		entwalletentry.EntryTypeEQ(entwalletentry.EntryTypeUsageReservation),
	).Only(ctx)
	if ent.IsNotFound(err) {
		return ErrRequestConflict
	}
	if err != nil {
		return fmt.Errorf("read usage reservation for refund: %w", err)
	}
	if reservation.AmountMicros != -amount {
		return ErrRequestConflict
	}
	if requireUnfinalized {
		usageExists, err := tx.APIUsage.Query().Where(entapiusage.RequestIDEQ(referenceID), entapiusage.StatusCodeLT(400)).Exist(ctx)
		if err != nil {
			return fmt.Errorf("check usage before releasing reservation: %w", err)
		}
		if usageExists {
			return nil
		}
	}
	existing, err := tx.WalletEntry.Query().Where(
		entwalletentry.WalletIDEQ(walletEntity.ID),
		entwalletentry.ReferenceIDEQ(referenceID),
		entwalletentry.EntryTypeEQ(entwalletentry.EntryTypeUsageRefund),
	).Only(ctx)
	if err == nil {
		if existing.AmountMicros == amount {
			return nil
		}
		return ErrRequestConflict
	}
	if !ent.IsNotFound(err) {
		return fmt.Errorf("read existing usage refund: %w", err)
	}
	if walletEntity.BalanceMicros > math.MaxInt64-amount {
		return ErrInvalidInput
	}
	next := walletEntity.BalanceMicros + amount
	if _, err := tx.Wallet.UpdateOneID(walletEntity.ID).SetBalanceMicros(next).Save(ctx); err != nil {
		return fmt.Errorf("refund usage balance: %w", err)
	}
	if _, err := tx.WalletEntry.Create().SetWalletID(walletEntity.ID).SetReferenceID(referenceID).SetEntryType(entwalletentry.EntryTypeUsageRefund).SetAmountMicros(amount).SetBalanceAfterMicros(next).SetDescription(limitDescription(description)).Save(ctx); err != nil {
		return fmt.Errorf("record usage refund: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit usage refund: %w", err)
	}
	return nil
}

func (s *EntStore) Finalize(ctx context.Context, input UsageInput) error {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin usage finalization: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	walletEntity, err := tx.Wallet.Query().Where(entwallet.UserIDEQ(input.UserID)).ForUpdate().Only(ctx)
	if ent.IsNotFound(err) {
		return ErrWalletNotFound
	}
	if err != nil {
		return fmt.Errorf("lock wallet for usage finalization: %w", err)
	}
	existingUsage, err := tx.APIUsage.Query().Where(entapiusage.RequestIDEQ(input.RequestID)).Only(ctx)
	if err == nil {
		if sameUsage(existingUsage, input) {
			return nil
		}
		return ErrRequestConflict
	}
	if !ent.IsNotFound(err) {
		return fmt.Errorf("read existing API usage: %w", err)
	}
	refund := input.ReservedMicros - input.CostMicros
	existingRefund, refundErr := tx.WalletEntry.Query().Where(
		entwalletentry.WalletIDEQ(walletEntity.ID),
		entwalletentry.ReferenceIDEQ(input.RequestID),
		entwalletentry.EntryTypeEQ(entwalletentry.EntryTypeUsageRefund),
	).Only(ctx)
	if refundErr == nil {
		if refund == 0 || existingRefund.AmountMicros != refund {
			return ErrRequestConflict
		}
	} else if ent.IsNotFound(refundErr) {
		if refund > 0 {
			if walletEntity.BalanceMicros > math.MaxInt64-refund {
				return ErrInvalidInput
			}
			next := walletEntity.BalanceMicros + refund
			if _, err := tx.Wallet.UpdateOneID(walletEntity.ID).SetBalanceMicros(next).Save(ctx); err != nil {
				return fmt.Errorf("release unused usage balance: %w", err)
			}
			if _, err := tx.WalletEntry.Create().SetWalletID(walletEntity.ID).SetReferenceID(input.RequestID).SetEntryType(entwalletentry.EntryTypeUsageRefund).SetAmountMicros(refund).SetBalanceAfterMicros(next).SetDescription("释放未使用的调用预占").Save(ctx); err != nil {
				return fmt.Errorf("record unused usage balance: %w", err)
			}
		}
	} else {
		return fmt.Errorf("read existing usage finalization refund: %w", refundErr)
	}
	if refund < 0 {
		extra := -refund
		existingSettlement, settlementErr := tx.WalletEntry.Query().Where(entwalletentry.WalletIDEQ(walletEntity.ID), entwalletentry.ReferenceIDEQ(input.RequestID), entwalletentry.EntryTypeEQ(entwalletentry.EntryTypeUsageSettlement)).Only(ctx)
		if settlementErr == nil {
			if existingSettlement.AmountMicros != -extra {
				return ErrRequestConflict
			}
		} else if ent.IsNotFound(settlementErr) {
			next := walletEntity.BalanceMicros - extra
			if _, err := tx.Wallet.UpdateOneID(walletEntity.ID).SetBalanceMicros(next).Save(ctx); err != nil {
				return fmt.Errorf("settle additional usage balance: %w", err)
			}
			if _, err := tx.WalletEntry.Create().SetWalletID(walletEntity.ID).SetReferenceID(input.RequestID).SetEntryType(entwalletentry.EntryTypeUsageSettlement).SetAmountMicros(-extra).SetBalanceAfterMicros(next).SetDescription("补扣实际调用费用").Save(ctx); err != nil {
				return fmt.Errorf("record additional usage balance: %w", err)
			}
		} else {
			return fmt.Errorf("read existing usage settlement: %w", settlementErr)
		}
	}
	create := tx.APIUsage.Create().SetUserID(input.UserID).SetAPIKeyID(input.APIKeyID).SetModelRouteID(input.ModelRouteID).SetUpstreamModelID(*input.UpstreamModelID).SetBillingGroupID(*input.BillingGroupID).
		SetRequestID(input.RequestID).SetEndpoint(entapiusage.Endpoint(input.Endpoint)).SetStatusCode(input.StatusCode).SetInputTokens(input.InputTokens).
		SetUncachedInputTokens(input.Tokens.UncachedInput).SetCacheReadInputTokens(input.Tokens.CacheRead).SetCacheWriteInputTokens(input.Tokens.CacheWrite).SetCacheWrite1hInputTokens(input.Tokens.CacheWrite1h).SetOutputTokens(input.OutputTokens).
		SetInputPriceMicros(input.Rates.InputMicros).SetOutputPriceMicros(input.Rates.OutputMicros).SetCacheReadPriceMicros(input.Rates.CacheReadMicros).SetCacheWritePriceMicros(input.Rates.CacheWriteMicros).SetCacheWrite1hPriceMicros(input.Rates.CacheWrite1hMicros).SetRequestPriceMicros(input.Rates.RequestMicros).
		SetBaseCostMicros(input.BaseCostMicros).SetMultiplierBps(input.MultiplierBPS).SetCostMicros(input.CostMicros).SetReservedMicros(input.ReservedMicros).SetEstimated(input.Estimated).
		SetUpstreamRequestID(input.UpstreamRequestID).SetModelName(input.ModelName).SetUpstreamModelName(input.UpstreamModelName).SetBillingGroupCode(input.BillingGroupCode).SetBillingGroupName(input.BillingGroupName).SetCalculationVersion(input.CalculationVersion).SetCreatedAt(input.CreatedAt).SetFinishedAt(input.FinishedAt).SetDurationMs(durationMilliseconds(input.CreatedAt, input.FinishedAt))
	if _, err := create.Save(ctx); err != nil {
		return fmt.Errorf("record API usage: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit usage finalization: %w", err)
	}
	return nil
}

func durationMilliseconds(startedAt, finishedAt time.Time) int64 {
	duration := finishedAt.Sub(startedAt).Milliseconds()
	if duration < 0 {
		return 0
	}
	return duration
}

func (s *EntStore) RecordFailure(ctx context.Context, input FailureInput) error {
	existing, err := s.client.APIUsage.Query().Where(entapiusage.RequestIDEQ(input.RequestID)).Only(ctx)
	if err == nil {
		if existing.UserID == input.UserID && existing.APIKeyID == input.APIKeyID && existing.StatusCode == input.StatusCode && existing.ErrorCode == input.ErrorCode && existing.ErrorMessage == input.ErrorMessage {
			return nil
		}
		return ErrRequestConflict
	}
	if !ent.IsNotFound(err) {
		return fmt.Errorf("read existing failed usage: %w", err)
	}
	create := s.client.APIUsage.Create().SetUserID(input.UserID).SetAPIKeyID(input.APIKeyID).SetModelRouteID(input.ModelRouteID).SetUpstreamModelID(*input.UpstreamModelID).SetBillingGroupID(*input.BillingGroupID).
		SetRequestID(input.RequestID).SetEndpoint(entapiusage.Endpoint(input.Endpoint)).SetStatusCode(input.StatusCode).SetErrorCode(input.ErrorCode).SetErrorMessage(input.ErrorMessage).
		SetMultiplierBps(input.MultiplierBPS).SetModelName(input.ModelName).SetUpstreamModelName(input.UpstreamModelName).SetBillingGroupCode(input.BillingGroupCode).SetBillingGroupName(input.BillingGroupName).
		SetCreatedAt(input.CreatedAt).SetFinishedAt(input.FinishedAt).SetDurationMs(input.DurationMS)
	if _, err := create.Save(ctx); err != nil {
		return fmt.Errorf("record failed API usage: %w", err)
	}
	return nil
}

func sameUsage(existing *ent.APIUsage, input UsageInput) bool {
	return existing != nil && existing.UserID == input.UserID && existing.APIKeyID == input.APIKeyID &&
		existing.ModelRouteID == input.ModelRouteID && string(existing.Endpoint) == input.Endpoint &&
		existing.StatusCode == input.StatusCode && existing.ErrorCode == "" && existing.ErrorMessage == "" &&
		existing.UpstreamModelID != nil && input.UpstreamModelID != nil && *existing.UpstreamModelID == *input.UpstreamModelID && existing.BillingGroupID != nil && input.BillingGroupID != nil && *existing.BillingGroupID == *input.BillingGroupID &&
		existing.InputTokens == input.InputTokens && existing.UncachedInputTokens == input.Tokens.UncachedInput && existing.CacheReadInputTokens == input.Tokens.CacheRead && existing.CacheWriteInputTokens == input.Tokens.CacheWrite && existing.CacheWrite1hInputTokens == input.Tokens.CacheWrite1h && existing.OutputTokens == input.OutputTokens &&
		existing.InputPriceMicros == input.Rates.InputMicros && existing.OutputPriceMicros == input.Rates.OutputMicros && existing.CacheReadPriceMicros == input.Rates.CacheReadMicros && existing.CacheWritePriceMicros == input.Rates.CacheWriteMicros && existing.CacheWrite1hPriceMicros == input.Rates.CacheWrite1hMicros && existing.RequestPriceMicros == input.Rates.RequestMicros && existing.BaseCostMicros == input.BaseCostMicros && existing.MultiplierBps == input.MultiplierBPS && existing.CostMicros == input.CostMicros && existing.ReservedMicros == input.ReservedMicros &&
		existing.Estimated == input.Estimated && existing.UpstreamRequestID == input.UpstreamRequestID
}

func walletFromEnt(entity *ent.Wallet) Wallet {
	return Wallet{ID: entity.ID, UserID: entity.UserID, BalanceMicros: entity.BalanceMicros, UpdatedAt: entity.UpdatedAt}
}
func entriesFromEnt(entities []*ent.WalletEntry) []Entry {
	entries := make([]Entry, 0, len(entities))
	for _, entity := range entities {
		entries = append(entries, Entry{ID: entity.ID, ReferenceID: entity.ReferenceID, Type: EntryType(entity.EntryType), AmountMicros: entity.AmountMicros, BalanceAfterMicros: entity.BalanceAfterMicros, Description: entity.Description, CreatedAt: entity.CreatedAt})
	}
	return entries
}

func limitDescription(value string) string {
	if utf8.RuneCountInString(value) <= 255 {
		return value
	}
	runes := []rune(value)
	return string(runes[:255])
}
