package payment

import (
	"context"
	"fmt"
	"math"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/ent"
	enttopuporder "github.com/novro-gateway/novro/ent/topuporder"
	entuser "github.com/novro-gateway/novro/ent/user"
	entwallet "github.com/novro-gateway/novro/ent/wallet"
	entwalletentry "github.com/novro-gateway/novro/ent/walletentry"
)

type EntStore struct{ client *ent.Client }

func NewEntStore(client *ent.Client) *EntStore { return &EntStore{client: client} }

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

func (s *EntStore) List(ctx context.Context, userID uuid.UUID, limit int) ([]Order, error) {
	entities, err := s.client.TopUpOrder.Query().Where(enttopuporder.UserIDEQ(userID)).Order(ent.Desc(enttopuporder.FieldCreatedAt)).Limit(limit).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list top-up orders: %w", err)
	}
	orders := make([]Order, 0, len(entities))
	for _, entity := range entities {
		orders = append(orders, orderFromEnt(entity))
	}
	return orders, nil
}

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

func orderFromEnt(entity *ent.TopUpOrder) Order {
	return Order{
		ID: entity.ID, UserID: entity.UserID, OutTradeNo: entity.OutTradeNo, Provider: string(entity.Provider),
		Channel: entity.Channel, AmountMicros: entity.AmountMicros, CreditedMicros: entity.CreditedMicros, Status: Status(entity.Status),
		ProviderTradeNo: entity.ProviderTradeNo, PaidAt: entity.PaidAt, CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt,
	}
}

func topUpDescription(amountMicros, creditedMicros int64) string {
	if creditedMicros > amountMicros {
		return "易支付充值（含赠送）"
	}
	return "易支付充值"
}
