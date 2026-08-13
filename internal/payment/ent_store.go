package payment

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/ent"
	entsystemsetting "github.com/novro-gateway/novro/ent/systemsetting"
	enttopuporder "github.com/novro-gateway/novro/ent/topuporder"
	entuser "github.com/novro-gateway/novro/ent/user"
	entwallet "github.com/novro-gateway/novro/ent/wallet"
	entwalletentry "github.com/novro-gateway/novro/ent/walletentry"
	"github.com/novro-gateway/novro/internal/referral"
)

type EntStore struct {
	client                   *ent.Client
	defaultReferralRewardBPS int64
}

/**
 * NewEntStore 用于创建并返回所需的对象或记录。
 * @param client 用于访问外部或底层服务的客户端。
 * @param referralRewardBPS 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func NewEntStore(client *ent.Client, referralRewardBPS ...int64) *EntStore {
	rate := int64(0)
	if len(referralRewardBPS) > 0 && referralRewardBPS[0] >= 0 && referralRewardBPS[0] <= 10_000 {
		rate = referralRewardBPS[0]
	}
	return &EntStore{client: client, defaultReferralRewardBPS: rate}
}

/**
 * Create 用于创建并返回所需的对象或记录。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param params 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) Create(ctx context.Context, params CreateParams) (Order, error) {
	entity, err := s.client.TopUpOrder.Create().
		SetID(params.ID).
		SetUserID(params.UserID).
		SetOutTradeNo(params.OutTradeNo).
		SetProvider(enttopuporder.ProviderEpay).
		SetChannel(params.Channel).
		SetAmountMicros(params.AmountMicros).
		SetCreditedMicros(params.CreditedMicros).
		Save(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("insert top-up order: %w", err)
	}
	return orderFromEnt(entity), nil
}

/**
 * Get 用于查询并返回所需的数据。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param outTradeNo 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) Get(ctx context.Context, outTradeNo string) (Order, error) {
	entity, err := s.client.TopUpOrder.Query().Where(enttopuporder.OutTradeNoEQ(outTradeNo)).Only(ctx)
	if ent.IsNotFound(err) {
		return Order{}, ErrOrderNotFound
	}
	if err != nil {
		return Order{}, fmt.Errorf("read top-up order: %w", err)
	}
	return orderFromEnt(entity), nil
}

/**
 * ListAll 用于筛选并返回数据列表。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param filter 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) ListAll(ctx context.Context, filter AdminListFilter) (AdminPage, error) {
	query := s.client.TopUpOrder.Query()
	if filter.Search != "" {
		query = query.Where(enttopuporder.Or(
			enttopuporder.OutTradeNoContainsFold(filter.Search),
			enttopuporder.ProviderTradeNoContainsFold(filter.Search),
			enttopuporder.HasUserWith(entuser.Or(
				entuser.UsernameContainsFold(filter.Search),
				entuser.DisplayNameContainsFold(filter.Search),
				entuser.EmailContainsFold(filter.Search),
			)),
		))
	}
	if filter.Status != "" {
		query = query.Where(enttopuporder.StatusEQ(enttopuporder.Status(filter.Status)))
	}
	if filter.Channel != "" {
		query = query.Where(enttopuporder.ChannelEQ(filter.Channel))
	}
	total, err := query.Clone().Count(ctx)
	if err != nil {
		return AdminPage{}, fmt.Errorf("count top-up orders: %w", err)
	}
	entities, err := query.WithUser().Order(ent.Desc(enttopuporder.FieldCreatedAt)).Offset(filter.Offset).Limit(filter.Limit).All(ctx)
	if err != nil {
		return AdminPage{}, fmt.Errorf("list all top-up orders: %w", err)
	}
	orders := make([]AdminOrder, 0, len(entities))
	for _, entity := range entities {
		owner, err := entity.Edges.UserOrErr()
		if err != nil {
			return AdminPage{}, fmt.Errorf("read top-up order owner: %w", err)
		}
		orders = append(orders, AdminOrder{
			Order: orderFromEnt(entity),
			Owner: TopUpOwner{ID: owner.ID, Username: owner.Username, DisplayName: owner.DisplayName},
		})
	}
	return AdminPage{Orders: orders, Total: total, Offset: filter.Offset, Limit: filter.Limit}, nil
}

/**
 * List 用于筛选并返回数据列表。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param userID 目标用户的唯一标识。
 * @param filter 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) List(ctx context.Context, userID uuid.UUID, filter ListFilter) (Page, error) {
	query := s.client.TopUpOrder.Query().Where(enttopuporder.UserIDEQ(userID))
	total, err := query.Clone().Count(ctx)
	if err != nil {
		return Page{}, fmt.Errorf("count top-up orders: %w", err)
	}
	entities, err := query.Order(ent.Desc(enttopuporder.FieldCreatedAt)).Offset(filter.Offset).Limit(filter.Limit).All(ctx)
	if err != nil {
		return Page{}, fmt.Errorf("list top-up orders: %w", err)
	}
	orders := make([]Order, 0, len(entities))
	for _, entity := range entities {
		orders = append(orders, orderFromEnt(entity))
	}
	return Page{Orders: orders, Total: total, Offset: filter.Offset, Limit: filter.Limit}, nil
}

/**
 * Complete 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param params 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) Complete(ctx context.Context, params CompleteParams) (Order, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("begin top-up completion: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	orderEntity, err := tx.TopUpOrder.Query().Where(enttopuporder.OutTradeNoEQ(params.OutTradeNo)).ForUpdate().Only(ctx)
	if ent.IsNotFound(err) {
		return Order{}, ErrOrderNotFound
	}
	if err != nil {
		return Order{}, fmt.Errorf("lock top-up order: %w", err)
	}
	if orderEntity.AmountMicros != params.AmountMicros || orderEntity.Channel != params.Channel {
		return Order{}, ErrOrderConflict
	}
	if orderEntity.Status == enttopuporder.StatusPaid {
		if orderEntity.ProviderTradeNo != nil && *orderEntity.ProviderTradeNo == params.ProviderTradeNo {
			return orderFromEnt(orderEntity), nil
		}
		return Order{}, ErrOrderConflict
	}

	walletEntity, err := tx.Wallet.Query().Where(entwallet.UserIDEQ(orderEntity.UserID)).ForUpdate().Only(ctx)
	if ent.IsNotFound(err) {
		return Order{}, ErrWalletNotFound
	}
	if err != nil {
		return Order{}, fmt.Errorf("lock wallet for top-up: %w", err)
	}
	creditedMicros := orderEntity.CreditedMicros
	if walletEntity.BalanceMicros > math.MaxInt64-creditedMicros {
		return Order{}, ErrOrderConflict
	}
	nextBalance := walletEntity.BalanceMicros + creditedMicros
	if _, err := tx.Wallet.UpdateOneID(walletEntity.ID).SetBalanceMicros(nextBalance).Save(ctx); err != nil {
		return Order{}, fmt.Errorf("credit top-up balance: %w", err)
	}
	if _, err := tx.WalletEntry.Create().
		SetWalletID(walletEntity.ID).
		SetReferenceID(orderEntity.ID).
		SetEntryType(entwalletentry.EntryTypeTopUp).
		SetAmountMicros(creditedMicros).
		SetBalanceAfterMicros(nextBalance).
		SetDescription(topUpDescription(orderEntity.AmountMicros, creditedMicros)).
		Save(ctx); err != nil {
		return Order{}, fmt.Errorf("record top-up balance: %w", err)
	}
	if err := s.creditReferralReward(ctx, tx, orderEntity.UserID, orderEntity.ID, orderEntity.AmountMicros); err != nil {
		return Order{}, err
	}
	updated, err := tx.TopUpOrder.UpdateOneID(orderEntity.ID).
		SetStatus(enttopuporder.StatusPaid).
		SetProviderTradeNo(params.ProviderTradeNo).
		SetPaidAt(params.PaidAt).
		Save(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("mark top-up order paid: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Order{}, fmt.Errorf("commit top-up completion: %w", err)
	}
	return orderFromEnt(updated), nil
}

/**
 * creditReferralReward 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param tx 本次操作需要使用的输入参数。
 * @param referredUserID 目标资源的一个或多个唯一标识。
 * @param orderID 目标资源的一个或多个唯一标识。
 * @param amountMicros 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) creditReferralReward(ctx context.Context, tx *ent.Tx, referredUserID, orderID uuid.UUID, amountMicros int64) error {
	referredUser, err := tx.User.Query().Where(entuser.IDEQ(referredUserID)).Only(ctx)
	if err != nil {
		return fmt.Errorf("read referred user for top-up: %w", err)
	}
	if referredUser.ReferredByUserID == nil {
		return nil
	}
	rewardBPS, err := s.referralRewardBPS(ctx, tx)
	if err != nil {
		return err
	}
	if rewardBPS <= 0 {
		return nil
	}
	if amountMicros > math.MaxInt64/rewardBPS {
		return ErrOrderConflict
	}
	rewardMicros := amountMicros * rewardBPS / 10_000
	if rewardMicros <= 0 {
		return nil
	}
	referrerWallet, err := tx.Wallet.Query().Where(entwallet.UserIDEQ(*referredUser.ReferredByUserID)).ForUpdate().Only(ctx)
	if ent.IsNotFound(err) {
		return ErrWalletNotFound
	}
	if err != nil {
		return fmt.Errorf("lock referrer wallet for cashback: %w", err)
	}
	if referrerWallet.BalanceMicros > math.MaxInt64-rewardMicros {
		return ErrOrderConflict
	}
	nextBalance := referrerWallet.BalanceMicros + rewardMicros
	if _, err := tx.Wallet.UpdateOneID(referrerWallet.ID).SetBalanceMicros(nextBalance).Save(ctx); err != nil {
		return fmt.Errorf("credit referral cashback: %w", err)
	}
	if _, err := tx.WalletEntry.Create().
		SetWalletID(referrerWallet.ID).
		SetReferenceID(orderID).
		SetEntryType(entwalletentry.EntryTypeReferralReward).
		SetAmountMicros(rewardMicros).
		SetBalanceAfterMicros(nextBalance).
		SetDescription("邀请好友充值返现").
		Save(ctx); err != nil {
		return fmt.Errorf("record referral cashback: %w", err)
	}
	return nil
}

/**
 * referralRewardBPS 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param tx 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) referralRewardBPS(ctx context.Context, tx *ent.Tx) (int64, error) {
	setting, err := tx.SystemSetting.Query().Where(entsystemsetting.IDEQ(referral.RewardBPSSettingKey)).Only(ctx)
	if ent.IsNotFound(err) {
		return s.defaultReferralRewardBPS, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read referral reward configuration: %w", err)
	}
	rewardBPS, err := strconv.ParseInt(strings.TrimSpace(setting.Value), 10, 64)
	if err != nil || !referral.ValidRewardBPS(rewardBPS) {
		return 0, fmt.Errorf("read referral reward configuration: invalid stored rate")
	}
	return rewardBPS, nil
}

/**
 * orderFromEnt 封装该名称对应的业务处理逻辑。
 * @param entity 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func orderFromEnt(entity *ent.TopUpOrder) Order {
	return Order{
		ID: entity.ID, UserID: entity.UserID, OutTradeNo: entity.OutTradeNo, Provider: string(entity.Provider),
		Channel: entity.Channel, AmountMicros: entity.AmountMicros, CreditedMicros: entity.CreditedMicros, Status: Status(entity.Status),
		ProviderTradeNo: entity.ProviderTradeNo, PaidAt: entity.PaidAt, CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt,
	}
}

/**
 * topUpDescription 封装该名称对应的业务处理逻辑。
 * @param amountMicros 本次操作需要使用的输入参数。
 * @param creditedMicros 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func topUpDescription(amountMicros, creditedMicros int64) string {
	if creditedMicros > amountMicros {
		return "易支付充值（含赠送）"
	}
	return "易支付充值"
}
