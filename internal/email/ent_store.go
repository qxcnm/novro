package email

import (
	"context"
	"fmt"

	"github.com/novro-gateway/novro/ent"
	entsmtp "github.com/novro-gateway/novro/ent/emailsmtpconfig"
)

type EntStore struct{ client *ent.Client }

func NewEntStore(client *ent.Client) *EntStore { return &EntStore{client: client} }

func (s *EntStore) Get(ctx context.Context) (StoredConfig, error) {
	entity, err := s.client.EmailSMTPConfig.Query().Where(entsmtp.IDEQ(configID)).Only(ctx)
	if ent.IsNotFound(err) {
		return StoredConfig{}, ErrConfigNotFound
	}
	if err != nil {
		return StoredConfig{}, fmt.Errorf("read SMTP configuration: %w", err)
	}
	return storedConfigFromEnt(entity), nil
}

func (s *EntStore) Upsert(ctx context.Context, input StoredConfigInput) (StoredConfig, error) {
	entity, err := s.client.EmailSMTPConfig.Get(ctx, configID)
	if ent.IsNotFound(err) {
		created, createErr := s.client.EmailSMTPConfig.Create().
			SetID(configID).SetEnabled(input.Enabled).SetHost(input.Host).SetPort(input.Port).
			SetUsername(input.Username).SetEncryptedPassword(input.EncryptedPassword).
			SetFromAddress(input.FromAddress).SetSecurity(input.Security).Save(ctx)
		if createErr == nil {
			return storedConfigFromEnt(created), nil
		}
		entity, err = s.client.EmailSMTPConfig.Get(ctx, configID)
		if err != nil {
			return StoredConfig{}, fmt.Errorf("save SMTP configuration: %w", createErr)
		}
	} else if err != nil {
		return StoredConfig{}, fmt.Errorf("read SMTP configuration before update: %w", err)
	}
	entity, err = entity.Update().
		SetEnabled(input.Enabled).SetHost(input.Host).SetPort(input.Port).
		SetUsername(input.Username).SetEncryptedPassword(input.EncryptedPassword).
		SetFromAddress(input.FromAddress).SetSecurity(input.Security).Save(ctx)
	if err != nil {
		return StoredConfig{}, fmt.Errorf("save SMTP configuration: %w", err)
	}
	return storedConfigFromEnt(entity), nil
}

func storedConfigFromEnt(entity *ent.EmailSMTPConfig) StoredConfig {
	return StoredConfig{ID: entity.ID, Enabled: entity.Enabled, Host: entity.Host, Port: entity.Port, Username: entity.Username, EncryptedPassword: entity.EncryptedPassword, FromAddress: entity.FromAddress, Security: entity.Security, CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt}
}
