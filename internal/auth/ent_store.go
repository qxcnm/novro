package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/novro-gateway/novro/ent"
	entbillinggroup "github.com/novro-gateway/novro/ent/billinggroup"
	entuser "github.com/novro-gateway/novro/ent/user"
	"github.com/novro-gateway/novro/ent/useridentity"
	"github.com/novro-gateway/novro/ent/usersession"
	"github.com/novro-gateway/novro/internal/user"
)

type EntStore struct {
	client *ent.Client
}

func NewEntStore(client *ent.Client) *EntStore {
	return &EntStore{client: client}
}

func (s *EntStore) FindUserByUsername(ctx context.Context, username string) (LoginUser, error) {
	entity, err := s.client.User.Query().Where(entuser.Or(entuser.UsernameEQ(username), entuser.EmailEQ(username))).WithBillingGroup().Only(ctx)
	if ent.IsNotFound(err) {
		return LoginUser{}, user.ErrNotFound
	}
	if err != nil {
		return LoginUser{}, fmt.Errorf("query user by username: %w", err)
	}
	return LoginUser{User: authUserFromEnt(entity), PasswordHash: entity.PasswordHash}, nil
}

var oidcUsernameInvalid = regexp.MustCompile(`[^a-z0-9._-]+`)

func (s *EntStore) FindOrCreateOIDCUser(ctx context.Context, identity OIDCUser, autoRegister bool) (user.Record, error) {
	entity, err := s.client.UserIdentity.Query().
		Where(useridentity.IssuerEQ(identity.Issuer), useridentity.SubjectEQ(identity.Subject)).
		WithUser(func(query *ent.UserQuery) { query.WithBillingGroup() }).Only(ctx)
	if err == nil {
		linked, edgeErr := entity.Edges.UserOrErr()
		if edgeErr != nil {
			return user.Record{}, user.ErrNotFound
		}
		return authUserFromEnt(linked), nil
	}
	if !ent.IsNotFound(err) {
		return user.Record{}, fmt.Errorf("query OIDC identity: %w", err)
	}
	if !autoRegister {
		return user.Record{}, ErrOIDCNotProvisioned
	}
	email, ok := user.NormalizeEmail(identity.Email)
	if !ok {
		return user.Record{}, ErrOIDCNotProvisioned
	}

	base := normalizeOIDCUsername(identity.PreferredUsername)
	if base == "" {
		base = normalizeOIDCUsername(strings.Split(identity.Email, "@")[0])
	}
	digest := sha256.Sum256([]byte(identity.Issuer + "\x00" + identity.Subject))
	suffix := hex.EncodeToString(digest[:])[:10]
	if len(base) > 52 {
		base = base[:52]
	}
	if len(base) < 3 {
		base = "oidc"
	}
	username := base + "-" + suffix

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return user.Record{}, fmt.Errorf("begin OIDC registration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	group, err := tx.BillingGroup.Query().Where(entbillinggroup.IsDefaultEQ(true), entbillinggroup.StatusEQ(entbillinggroup.StatusActive), entbillinggroup.DeletedAtIsNil()).Only(ctx)
	if err != nil {
		return user.Record{}, fmt.Errorf("resolve OIDC billing group: %w", err)
	}
	created, err := tx.User.Create().
		SetBillingGroupID(group.ID).
		SetUsername(username).
		SetEmail(email).
		SetDisplayName(strings.TrimSpace(identity.DisplayName)).
		SetRole(entuser.RoleMember).
		SetStatus(entuser.StatusActive).
		Save(ctx)
	if ent.IsConstraintError(err) {
		return user.Record{}, ErrOIDCNotProvisioned
	}
	if err != nil {
		return user.Record{}, fmt.Errorf("create OIDC user: %w", err)
	}
	if _, err := tx.UserIdentity.Create().
		SetUserID(created.ID).
		SetIssuer(identity.Issuer).
		SetSubject(identity.Subject).
		SetEmail(email).
		Save(ctx); err != nil {
		if ent.IsConstraintError(err) {
			return user.Record{}, ErrOIDCNotProvisioned
		}
		return user.Record{}, fmt.Errorf("create OIDC identity: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return user.Record{}, fmt.Errorf("commit OIDC registration: %w", err)
	}
	return authUserFromEnt(created), nil
}

func normalizeOIDCUsername(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = oidcUsernameInvalid.ReplaceAllString(value, "-")
	return strings.Trim(value, "-._")
}

func (s *EntStore) CreateSession(
	ctx context.Context,
	userID uuid.UUID,
	tokenHash string,
	expiresAt time.Time,
	now time.Time,
) error {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin login transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	entity, err := tx.User.Query().
		Where(entuser.IDEQ(userID), entuser.StatusEQ(entuser.StatusActive)).
		ForUpdate().
		Only(ctx)
	if ent.IsNotFound(err) {
		return user.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock login user: %w", err)
	}
	if _, err := tx.UserSession.Create().
		SetUserID(entity.ID).
		SetTokenHash(tokenHash).
		SetExpiresAt(expiresAt).
		SetCreatedAt(now).
		SetLastSeenAt(now).
		Save(ctx); err != nil {
		return fmt.Errorf("insert login session: %w", err)
	}
	if _, err := tx.User.UpdateOneID(entity.ID).SetLastLoginAt(now).Save(ctx); err != nil {
		return fmt.Errorf("update last login: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit login: %w", err)
	}
	return nil
}

func (s *EntStore) FindUserBySession(ctx context.Context, tokenHash string, now time.Time) (user.Record, error) {
	session, err := s.client.UserSession.Query().
		Where(
			usersession.TokenHashEQ(tokenHash),
			usersession.RevokedAtIsNil(),
			usersession.ExpiresAtGT(now),
			usersession.HasUserWith(entuser.StatusEQ(entuser.StatusActive)),
		).
		WithUser(func(query *ent.UserQuery) { query.WithBillingGroup() }).
		Only(ctx)
	if ent.IsNotFound(err) {
		return user.Record{}, user.ErrNotFound
	}
	if err != nil {
		return user.Record{}, fmt.Errorf("query session: %w", err)
	}
	entity, err := session.Edges.UserOrErr()
	if err != nil {
		return user.Record{}, user.ErrNotFound
	}
	if _, err := s.client.UserSession.UpdateOneID(session.ID).SetLastSeenAt(now).Save(ctx); err != nil {
		return user.Record{}, fmt.Errorf("touch session: %w", err)
	}
	return authUserFromEnt(entity), nil
}

func (s *EntStore) RevokeSession(ctx context.Context, tokenHash string, now time.Time) error {
	_, err := s.client.UserSession.Update().
		Where(usersession.TokenHashEQ(tokenHash), usersession.RevokedAtIsNil()).
		SetRevokedAt(now).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func authUserFromEnt(entity *ent.User) user.Record {
	record := user.Record{
		ID:             entity.ID,
		BillingGroupID: entity.BillingGroupID,
		Username:       entity.Username,
		DisplayName:    entity.DisplayName,
		Role:           user.Role(entity.Role),
		Status:         user.Status(entity.Status),
		LastLoginAt:    entity.LastLoginAt,
		CreatedAt:      entity.CreatedAt,
		UpdatedAt:      entity.UpdatedAt,
	}
	if entity.Email != nil {
		record.Email = *entity.Email
	}
	if group, err := entity.Edges.BillingGroupOrErr(); err == nil {
		record.BillingGroup = &user.BillingGroupSummary{ID: group.ID, Code: group.Code, DisplayName: group.DisplayName, MultiplierBPS: group.MultiplierBps}
	}
	return record
}
