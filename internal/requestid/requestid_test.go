package requestid

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestMiddlewareGeneratesAndReusesRequestID(t *testing.T) {
	wanted := uuid.New()
	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := FromContext(r.Context()); got != wanted {
			t.Fatalf("context request id=%s want=%s", got, wanted)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request = request.WithContext(WithContext(request.Context(), wanted))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if got := response.Header().Get(Header); got != wanted.String() {
		t.Fatalf("request id header=%q want=%q", got, wanted)
	}
}

func TestMiddlewareIgnoresClientRequestID(t *testing.T) {
	clientID := uuid.New().String()
	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := FromContext(r.Context()); got == uuid.Nil || got.String() == clientID {
			t.Fatalf("unexpected generated request id %s", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(Header, clientID)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if got := response.Header().Get(Header); got == "" || got == clientID {
		t.Fatalf("request id header=%q", got)
	}
}
