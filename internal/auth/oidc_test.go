package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/novro-gateway/novro/internal/config"
	"golang.org/x/oauth2"
)

func TestOIDCClientAuthorizationCodeFlow(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate signing key: %v", err)
	}
	const keyID = "test-key"
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: privateKey},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", keyID),
	)
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}

	var issuer, expectedChallenge, expectedNonce string
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			writeTestJSON(t, w, map[string]any{
				"issuer": issuer, "authorization_endpoint": issuer + "/authorize",
				"token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys",
				"response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"},
				"id_token_signing_alg_values_supported": []string{"RS256"},
				"token_endpoint_auth_methods_supported": []string{"client_secret_post"},
			})
		case "/keys":
			writeTestJSON(t, w, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
				Key: &privateKey.PublicKey, KeyID: keyID, Algorithm: string(jose.RS256), Use: "sig",
			}}})
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse token request: %v", err)
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}
			challenge := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if got := base64.RawURLEncoding.EncodeToString(challenge[:]); got != expectedChallenge {
				t.Errorf("PKCE challenge mismatch: got %q want %q", got, expectedChallenge)
				http.Error(w, "invalid verifier", http.StatusBadRequest)
				return
			}
			now := time.Now().UTC()
			rawIDToken, signErr := jwt.Signed(signer).Claims(map[string]any{
				"iss": issuer, "sub": "subject-123", "aud": "novro-test", "nonce": expectedNonce,
				"email": "member@example.com", "preferred_username": "member.one", "name": "Member One",
				"iat": now.Unix(), "exp": now.Add(time.Minute).Unix(),
			}).Serialize()
			if signErr != nil {
				t.Errorf("sign ID token: %v", signErr)
				http.Error(w, "signing failed", http.StatusInternalServerError)
				return
			}
			writeTestJSON(t, w, map[string]any{
				"access_token": "provider-access-token", "token_type": "Bearer", "expires_in": 60, "id_token": rawIDToken,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()
	issuer = provider.URL

	ctx := oidc.ClientContext(context.Background(), provider.Client())
	client, err := NewOIDCClient(ctx, config.OIDCConfig{
		Issuer: issuer, ClientID: "novro-test", ClientSecret: "test-client-secret", AutoRegister: true,
	}, "https://app.example.invalid", "01234567890123456789012345678901")
	if err != nil {
		t.Fatalf("create OIDC client: %v", err)
	}
	flow, err := client.Start()
	if err != nil {
		t.Fatalf("start OIDC flow: %v", err)
	}
	authorizationURL, err := url.Parse(flow.AuthorizationURL)
	if err != nil {
		t.Fatalf("parse authorization URL: %v", err)
	}
	expectedChallenge = authorizationURL.Query().Get("code_challenge")
	expectedNonce = authorizationURL.Query().Get("nonce")
	state := authorizationURL.Query().Get("state")
	if expectedChallenge == "" || expectedNonce == "" || state == "" || authorizationURL.Query().Get("code_challenge_method") != "S256" {
		t.Fatalf("authorization URL lacks protected flow parameters: %s", authorizationURL.RawQuery)
	}

	identity, autoRegister, err := client.Complete(ctx, "authorization-code", state, flow.CookieValue)
	if err != nil {
		t.Fatalf("complete OIDC flow: %v", err)
	}
	if !autoRegister || identity.Issuer != issuer || identity.Subject != "subject-123" ||
		identity.Email != "member@example.com" || identity.PreferredUsername != "member.one" || identity.DisplayName != "Member One" {
		t.Fatalf("unexpected identity: %+v auto_register=%v", identity, autoRegister)
	}
	if _, _, err := client.Complete(ctx, "authorization-code", "wrong-state", flow.CookieValue); err != ErrInvalidOIDCFlow {
		t.Fatalf("wrong state was not rejected: %v", err)
	}
}

func TestOIDCClientRejectsExpiredAndTamperedFlowState(t *testing.T) {
	client := testOIDCStateClient(t)
	client.now = func() time.Time { return time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC) }
	flow, err := client.Start()
	if err != nil {
		t.Fatalf("start OIDC flow: %v", err)
	}
	client.now = func() time.Time { return flow.ExpiresAt.Add(time.Second) }
	state, err := client.decryptState(flow.CookieValue)
	if err != nil {
		t.Fatalf("decrypt flow state: %v", err)
	}
	if _, _, err := client.Complete(context.Background(), "code", state.State, flow.CookieValue); err != ErrInvalidOIDCFlow {
		t.Fatalf("expired flow was not rejected: %v", err)
	}
	replacement := byte('A')
	if flow.CookieValue[0] == replacement {
		replacement = 'B'
	}
	tampered := string(replacement) + flow.CookieValue[1:]
	if _, err := client.decryptState(tampered); err != ErrInvalidOIDCFlow {
		t.Fatalf("tampered flow cookie was not rejected: %v", err)
	}
}

func testOIDCStateClient(t *testing.T) *OIDCClient {
	t.Helper()
	key := sha256.Sum256([]byte("test OIDC state key"))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		t.Fatalf("create test cipher: %v", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("create test AEAD: %v", err)
	}
	return &OIDCClient{
		issuer: "https://id.example.com", autoRegister: true,
		oauth: oauth2.Config{
			ClientID: "novro-test", RedirectURL: "https://app.example.invalid/api/auth/oidc/callback",
			Endpoint: oauth2.Endpoint{AuthURL: "https://id.example.com/authorize", TokenURL: "https://id.example.com/token"},
		},
		aead: aead,
	}
}

func writeTestJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Errorf("write test response: %v", err)
	}
}
