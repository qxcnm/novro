package main

import (
	"testing"

	"github.com/novro-gateway/novro/internal/auth"
)

func TestOptionalOIDCServicePreservesDisabledState(t *testing.T) {
	if service := optionalOIDCService(nil); service != nil {
		t.Fatal("disabled OIDC client must remain a nil service")
	}

	client := &auth.OIDCClient{}
	if service := optionalOIDCService(client); service == nil {
		t.Fatal("configured OIDC client must remain available")
	}
}
