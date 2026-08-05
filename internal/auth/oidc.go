package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/novro-gateway/novro/internal/config"
	"golang.org/x/oauth2"
)

const oidcFlowTTL = 10 * time.Minute

var ErrInvalidOIDCFlow = errors.New("invalid OIDC flow")

type OIDCFlow struct {
	AuthorizationURL string
	CookieValue      string
	ExpiresAt        time.Time
}

type oidcFlowState struct {
	State        string    `json:"state"`
	Nonce        string    `json:"nonce"`
	CodeVerifier string    `json:"code_verifier"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type OIDCClient struct {
	issuer       string
	autoRegister bool
	oauth        oauth2.Config
	verifier     *oidc.IDTokenVerifier
	aead         cipher.AEAD
	now          func() time.Time
}

func NewOIDCClient(ctx context.Context, cfg config.OIDCConfig, publicURL, sessionSecret string) (*OIDCClient, error) {
	if !cfg.Enabled() {
		return nil, nil
	}
	discoveryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	provider, err := oidc.NewProvider(discoveryCtx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("discover OIDC provider: %w", err)
	}
	key := sha256.Sum256([]byte("novro-oidc-flow:" + sessionSecret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("initialize OIDC flow encryption: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("initialize OIDC flow encryption: %w", err)
	}
	return &OIDCClient{
		issuer:       cfg.Issuer,
		autoRegister: cfg.AutoRegister,
		oauth: oauth2.Config{
			ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret,
			RedirectURL: publicURL + "/api/auth/oidc/callback",
			Endpoint:    provider.Endpoint(),
			Scopes:      []string{oidc.ScopeOpenID, oidc.ScopeProfile, oidc.ScopeEmail},
		},
		verifier: provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
		aead:     aead,
		now:      func() time.Time { return time.Now().UTC() },
	}, nil
}

func (c *OIDCClient) Start() (OIDCFlow, error) {
	state, err := randomURLSafe(32)
	if err != nil {
		return OIDCFlow{}, err
	}
	nonce, err := randomURLSafe(32)
	if err != nil {
		return OIDCFlow{}, err
	}
	verifier := oauth2.GenerateVerifier()
	expiresAt := c.now().Add(oidcFlowTTL)
	encoded, err := c.encryptState(oidcFlowState{State: state, Nonce: nonce, CodeVerifier: verifier, ExpiresAt: expiresAt})
	if err != nil {
		return OIDCFlow{}, err
	}
	return OIDCFlow{
		AuthorizationURL: c.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)),
		CookieValue:      encoded,
		ExpiresAt:        expiresAt,
	}, nil
}

func (c *OIDCClient) Complete(ctx context.Context, code, state, cookieValue string) (OIDCUser, bool, error) {
	flow, err := c.decryptState(cookieValue)
	if err != nil || flow.ExpiresAt.Before(c.now()) || state == "" || state != flow.State || code == "" {
		return OIDCUser{}, false, ErrInvalidOIDCFlow
	}
	token, err := c.oauth.Exchange(ctx, code, oauth2.VerifierOption(flow.CodeVerifier))
	if err != nil {
		return OIDCUser{}, false, ErrInvalidOIDCFlow
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return OIDCUser{}, false, ErrInvalidOIDCFlow
	}
	idToken, err := c.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return OIDCUser{}, false, ErrInvalidOIDCFlow
	}
	var claims struct {
		Subject           string `json:"sub"`
		Nonce             string `json:"nonce"`
		Email             string `json:"email"`
		PreferredUsername string `json:"preferred_username"`
		Name              string `json:"name"`
	}
	if err := idToken.Claims(&claims); err != nil || claims.Subject == "" || claims.Nonce != flow.Nonce {
		return OIDCUser{}, false, ErrInvalidOIDCFlow
	}
	return OIDCUser{
		Issuer: c.issuer, Subject: claims.Subject, Email: claims.Email,
		PreferredUsername: claims.PreferredUsername, DisplayName: claims.Name,
	}, c.autoRegister, nil
}

func (c *OIDCClient) encryptState(state oidcFlowState) (string, error) {
	plaintext, err := json.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("encode OIDC flow: %w", err)
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate OIDC flow nonce: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, plaintext, nil)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (c *OIDCClient) decryptState(encoded string) (oidcFlowState, error) {
	sealed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(sealed) <= c.aead.NonceSize() {
		return oidcFlowState{}, ErrInvalidOIDCFlow
	}
	nonce, ciphertext := sealed[:c.aead.NonceSize()], sealed[c.aead.NonceSize():]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return oidcFlowState{}, ErrInvalidOIDCFlow
	}
	var state oidcFlowState
	if err := json.Unmarshal(plaintext, &state); err != nil {
		return oidcFlowState{}, ErrInvalidOIDCFlow
	}
	return state, nil
}

func randomURLSafe(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate secure random value: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
