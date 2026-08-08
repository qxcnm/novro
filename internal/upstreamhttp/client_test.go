package upstreamhttp

import (
	"net/http"
	"testing"
)

func TestNewClientDoesNotSetModelCallTimeouts(t *testing.T) {
	client := NewClient()
	if client.Timeout != 0 {
		t.Fatalf("client timeout=%v", client.Timeout)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type=%T", client.Transport)
	}
	if transport.IdleConnTimeout != 0 || transport.TLSHandshakeTimeout != 0 || transport.ResponseHeaderTimeout != 0 {
		t.Fatalf("unexpected transport timeouts: idle=%v tls=%v response_header=%v", transport.IdleConnTimeout, transport.TLSHandshakeTimeout, transport.ResponseHeaderTimeout)
	}
}
