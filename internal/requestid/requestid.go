package requestid

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

const Header = "X-Novro-Request-ID"

type contextKey struct{}

func New() uuid.UUID {
	return uuid.New()
}

func WithContext(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

func FromContext(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(contextKey{}).(uuid.UUID)
	return id
}

func Ensure(ctx context.Context) (context.Context, uuid.UUID) {
	if id := FromContext(ctx); id != uuid.Nil {
		return ctx, id
	}
	id := New()
	return WithContext(ctx, id), id
}

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, id := Ensure(r.Context())
		w.Header().Set(Header, id.String())
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ResponseID(w http.ResponseWriter) uuid.UUID {
	id, err := uuid.Parse(w.Header().Get(Header))
	if err != nil {
		return uuid.Nil
	}
	return id
}
