package upstreamhttp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

func NewClient() *http.Client {
	dialer := &net.Dialer{Timeout: 0, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, fmt.Errorf("parse upstream address: %w", err)
			}
			addresses, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
			if err != nil {
				return nil, fmt.Errorf("resolve upstream host: %w", err)
			}
			for _, ip := range addresses {
				if UnsafeIP(ip) {
					continue
				}
				connection, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if dialErr == nil {
					return connection, nil
				}
				err = dialErr
			}
			if err != nil {
				return nil, fmt.Errorf("dial upstream host: %w", err)
			}
			return nil, fmt.Errorf("upstream host does not resolve to a public address")
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       0,
		TLSHandshakeTimeout:   0,
		ResponseHeaderTimeout: 0,
	}
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func UnsafeIP(ip net.IP) bool {
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}
