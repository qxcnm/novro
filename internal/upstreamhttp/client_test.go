package upstreamhttp

import (
	"crypto/tls"
	"net/http"
	"testing"
)

func TestNewClientLimitsConnectionSetupAndResponseHeaders(t *testing.T) {
	client := NewClient()
	if client.Timeout != 0 {
		t.Fatalf("client timeout=%v", client.Timeout)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type=%T", client.Transport)
	}
	if transport.IdleConnTimeout != 0 || transport.TLSHandshakeTimeout != tlsHandshakeTimeout || transport.ResponseHeaderTimeout != responseHeaderTimeout {
		t.Fatalf("unexpected transport timeouts: idle=%v tls=%v response_header=%v", transport.IdleConnTimeout, transport.TLSHandshakeTimeout, transport.ResponseHeaderTimeout)
	}
	if transport.ForceAttemptHTTP2 || transport.TLSNextProto == nil || len(transport.TLSNextProto) != 0 {
		t.Fatalf("upstream transport must stay on HTTP/1.1: force_http2=%v tls_next_proto=%v", transport.ForceAttemptHTTP2, transport.TLSNextProto)
	}
	if transport.TLSClientConfig == nil || transport.TLSClientConfig.MinVersion != tls.VersionTLS12 || transport.TLSClientConfig.MaxVersion != 0 || transport.TLSClientConfig.Renegotiation != tls.RenegotiateFreelyAsClient {
		t.Fatalf("upstream transport must allow TLS renegotiation with TLS 1.2 minimum: config=%+v", transport.TLSClientConfig)
	}
}
