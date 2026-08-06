package billing

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

type Store interface {
	GetSummary(context.Context, uuid.UUID, int) (Summary, error)
	ListUsage(context.Context, uuid.UUID, int) ([]Usage, error)
	Adjust(context.Context, uuid.UUID, uuid.UUID, int64, string) (Summary, error)
	Reserve(context.Context, uuid.UUID, uuid.UUID, int64, string) error
	Refund(context.Context, uuid.UUID, uuid.UUID, int64, string) error
	Finalize(context.Context, UsageInput) error
}

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

func (s *Service) Summary(ctx context.Context, userID uuid.UUID) (Summary, error) {
	if userID == uuid.Nil {
		return Summary{}, ErrInvalidInput
	}
	return s.store.GetSummary(ctx, userID, 20)
}

func (s *Service) Usage(ctx context.Context, userID uuid.UUID) ([]Usage, error) {
	if userID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	return s.store.ListUsage(ctx, userID, 50)
}

func (s *Service) Adjust(ctx context.Context, userID, actorID uuid.UUID, amountMicros int64, note string) (Summary, error) {
	note = strings.TrimSpace(note)
	if userID == uuid.Nil || actorID == uuid.Nil || amountMicros == 0 || amountMicros < -1_000_000_000_000_000 || amountMicros > 1_000_000_000_000_000 || note == "" || len([]rune(note)) > 255 {
		return Summary{}, ErrInvalidInput
	}
	return s.store.Adjust(ctx, userID, actorID, amountMicros, note)
}

func (s *Service) Reserve(ctx context.Context, userID, referenceID uuid.UUID, amountMicros int64, description string) error {
	if userID == uuid.Nil || referenceID == uuid.Nil || amountMicros <= 0 || amountMicros > 1_000_000_000_000_000 {
		return ErrInvalidInput
	}
	return s.store.Reserve(ctx, userID, referenceID, amountMicros, description)
}

func (s *Service) Refund(ctx context.Context, userID, referenceID uuid.UUID, amountMicros int64, description string) error {
	if userID == uuid.Nil || referenceID == uuid.Nil || amountMicros <= 0 || amountMicros > 1_000_000_000_000_000 {
		return ErrInvalidInput
	}
	return s.store.Refund(ctx, userID, referenceID, amountMicros, description)
}

func (s *Service) Finalize(ctx context.Context, input UsageInput) error {
	if input.UserID == uuid.Nil || input.APIKeyID == uuid.Nil || input.ModelRouteID == uuid.Nil || input.RequestID == uuid.Nil || input.InputTokens < 0 || input.OutputTokens < 0 || input.CostMicros < 0 || input.ReservedMicros < input.CostMicros || input.CostMicros > 1_000_000_000_000_000 || input.ReservedMicros > 1_000_000_000_000_000 || (input.Endpoint != "chat_completions" && input.Endpoint != "responses" && input.Endpoint != "messages") {
		return ErrInvalidInput
	}
	return s.store.Finalize(ctx, input)
}
