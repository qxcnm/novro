package apikey

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/ent"
	entapikey "github.com/novro-gateway/novro/ent/apikey"
	entuser "github.com/novro-gateway/novro/ent/user"
	"github.com/novro-gateway/novro/internal/user"
)

type EntStore struct {
	client *ent.Client
}

func (s *EntStore) AuthenticateHash(ctx context.Context, hash string, now time.Time) (Actor, error) {
	entity, err := s.client.APIKey.Query().
		Where(entapikey.KeyHashEQ(hash), entapikey.StatusEQ(entapikey.StatusActive), entapikey.HasUserWith(entuser.StatusEQ(entuser.StatusActive))).
		WithUser().Only(ctx)
	if ent.IsNotFound(err) {
		return Actor{}, ErrUnauthenticated
	}
	if err != nil {
		return Actor{}, fmt.Errorf("authenticate API key: %w", err)
	}
	owner, err := entity.Edges.UserOrErr()
	if err != nil {
		return Actor{}, fmt.Errorf("read API key owner: %w", err)
	}
	if _, err := s.client.APIKey.UpdateOneID(entity.ID).SetLastUsedAt(now).Save(ctx); err != nil {
		return Actor{}, fmt.Errorf("update API key last use: %w", err)
	}
	return Actor{APIKey: fromEnt(entity), User: user.Record{ID: owner.ID, Username: owner.Username, DisplayName: owner.DisplayName, Role: user.Role(owner.Role), Status: user.Status(owner.Status), LastLoginAt: owner.LastLoginAt, CreatedAt: owner.CreatedAt, UpdatedAt: owner.UpdatedAt}}, nil
}

func NewEntStore(client *ent.Client) *EntStore {
	return &EntStore{client: client}
}

func (s *EntStore) Create(ctx context.Context, userID uuid.UUID, name, prefix, hash string, maxActive int) (Record, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return Record{}, fmt.Errorf("begin API key creation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.User.Query().Where(entuser.IDEQ(userID), entuser.StatusEQ(entuser.StatusActive)).ForUpdate().Only(ctx); ent.IsNotFound(err) {
		return Record{}, ErrNotFound
	} else if err != nil {
		return Record{}, fmt.Errorf("lock API key owner: %w", err)
	}
	count, err := tx.APIKey.Query().Where(entapikey.UserIDEQ(userID), entapikey.StatusEQ(entapikey.StatusActive)).Count(ctx)
	if err != nil {
		return Record{}, fmt.Errorf("count active API keys: %w", err)
	}
	if count >= maxActive {
		return Record{}, ErrLimitReached
	}
	created, err := tx.APIKey.Create().
		SetUserID(userID).
		SetName(name).
		SetKeyPrefix(prefix).
		SetKeyHash(hash).
		SetStatus(entapikey.StatusActive).
		Save(ctx)
	if err != nil {
		return Record{}, fmt.Errorf("create API key: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Record{}, fmt.Errorf("commit API key creation: %w", err)
	}
	return fromEnt(created), nil
}

func (s *EntStore) ListByUser(ctx context.Context, userID uuid.UUID) ([]Record, error) {
	entities, err := s.client.APIKey.Query().
		Where(entapikey.UserIDEQ(userID)).
		Order(ent.Desc(entapikey.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list user API keys: %w", err)
	}
	records := make([]Record, 0, len(entities))
	for _, entity := range entities {
		records = append(records, fromEnt(entity))
	}
	return records, nil
}

func (s *EntStore) RevokeByUser(ctx context.Context, userID, id uuid.UUID, now time.Time) error {
	entity, err := s.client.APIKey.Query().Where(entapikey.IDEQ(id), entapikey.UserIDEQ(userID)).Only(ctx)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("find user API key: %w", err)
	}
	return s.revoke(ctx, entity, now)
}

func (s *EntStore) ListAll(ctx context.Context, filter ListFilter) (Page, error) {
	query := s.client.APIKey.Query()
	if filter.Search != "" {
		query = query.Where(entapikey.Or(
			entapikey.NameContainsFold(filter.Search),
			entapikey.KeyPrefixContainsFold(filter.Search),
			entapikey.HasUserWith(entuser.Or(
				entuser.UsernameContainsFold(filter.Search),
				entuser.DisplayNameContainsFold(filter.Search),
			)),
		))
	}
	if filter.Status != "" {
		query = query.Where(entapikey.StatusEQ(entapikey.Status(filter.Status)))
	}
	total, err := query.Clone().Count(ctx)
	if err != nil {
		return Page{}, fmt.Errorf("count API keys: %w", err)
	}
	entities, err := query.WithUser().
		Order(ent.Desc(entapikey.FieldCreatedAt)).
		Offset(filter.Offset).
		Limit(filter.Limit).
		All(ctx)
	if err != nil {
		return Page{}, fmt.Errorf("list API keys: %w", err)
	}
	records := make([]AdminRecord, 0, len(entities))
	for _, entity := range entities {
		owner, err := entity.Edges.UserOrErr()
		if err != nil {
			return Page{}, fmt.Errorf("read API key owner: %w", err)
		}
		records = append(records, AdminRecord{
			Record: fromEnt(entity),
			Owner:  Owner{ID: owner.ID, Username: owner.Username, DisplayName: owner.DisplayName},
		})
	}
	return Page{APIKeys: records, Total: total, Offset: filter.Offset, Limit: filter.Limit}, nil
}

func (s *EntStore) Revoke(ctx context.Context, id uuid.UUID, now time.Time) error {
	entity, err := s.client.APIKey.Get(ctx, id)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("find API key: %w", err)
	}
	return s.revoke(ctx, entity, now)
}

func (s *EntStore) revoke(ctx context.Context, entity *ent.APIKey, now time.Time) error {
	if entity.Status == entapikey.StatusRevoked {
		return nil
	}
	if _, err := entity.Update().SetStatus(entapikey.StatusRevoked).SetRevokedAt(now).Save(ctx); err != nil {
		return fmt.Errorf("revoke API key: %w", err)
	}
	return nil
}

func fromEnt(entity *ent.APIKey) Record {
	return Record{
		ID:         entity.ID,
		UserID:     entity.UserID,
		Name:       entity.Name,
		KeyPrefix:  entity.KeyPrefix,
		Status:     Status(entity.Status),
		LastUsedAt: entity.LastUsedAt,
		CreatedAt:  entity.CreatedAt,
		RevokedAt:  entity.RevokedAt,
	}
}
