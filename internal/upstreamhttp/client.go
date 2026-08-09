package upstreamhttp

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	connectionTimeout   = 15 * time.Second
	tlsHandshakeTimeout = 30 * time.Second
	// Kimi may spend more than a minute preparing the first response headers
	// before streaming or returning the completion body.
	responseHeaderTimeout = 180 * time.Second
	dialAttempts          = 3
	dialRetryDelay        = 750 * time.Millisecond
)

func NewClient() *http.Client {
	dialer := &net.Dialer{Timeout: connectionTimeout, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		TLSClientConfig: &tls.Config{
			MinVersion:    tls.VersionTLS12,
			Renegotiation: tls.RenegotiateFreelyAsClient,
		},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, fmt.Errorf("parse upstream address: %w", err)
			}
			addresses, err := resolveUpstreamIPs(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("resolve upstream host: %w", err)
			}
			for _, ip := range addresses {
				if UnsafeIP(ip) {
					continue
				}
				for attempt := 0; attempt < dialAttempts; attempt++ {
					connection, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
					if dialErr == nil {
						return connection, nil
					}
					err = dialErr
					if ctx.Err() != nil || attempt+1 == dialAttempts {
						break
					}
					timer := time.NewTimer(dialRetryDelay)
					select {
					case <-ctx.Done():
						timer.Stop()
						return nil, ctx.Err()
					case <-timer.C:
					}
				}
			}
			if err != nil {
				return nil, fmt.Errorf("dial upstream host: %w", err)
			}
			return nil, fmt.Errorf("upstream host does not resolve to a public address")
		},
		// Some compatible gateways advertise HTTP/2 but terminate requests during
		// TLS or stream setup. HTTP/1.1 avoids opaque EOF failures and remains
		// compatible with the supported upstream APIs.
		ForceAttemptHTTP2:   false,
		TLSNextProto:        make(map[string]func(string, *tls.Conn) http.RoundTripper),
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     0,
		TLSHandshakeTimeout: tlsHandshakeTimeout,
		// A provider that blackholes a connection must not hold a user request
		// until the downstream client gives up. Model generation can continue
		// streaming after headers arrive, so this only bounds connection setup.
		ResponseHeaderTimeout: responseHeaderTimeout,
	}
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func UnsafeIP(ip net.IP) bool {
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() || isSyntheticIP(ip)
}

var syntheticIPRange = &net.IPNet{IP: net.ParseIP("198.18.0.0").To4(), Mask: net.CIDRMask(15, 32)}

func isSyntheticIP(ip net.IP) bool {
	return syntheticIPRange.Contains(ip)
}

func resolveUpstreamIPs(ctx context.Context, host string) ([]net.IP, error) {
	addresses, lookupErr := net.DefaultResolver.LookupIP(ctx, "ip", host)
	public := make([]net.IP, 0, len(addresses))
	hasSynthetic := false
	for _, ip := range addresses {
		if isSyntheticIP(ip) {
			hasSynthetic = true
			continue
		}
		if !UnsafeIP(ip) {
			public = append(public, ip)
		}
	}
	if len(public) > 0 {
		return public, nil
	}
	if net.ParseIP(host) != nil || (!hasSynthetic && lookupErr == nil) {
		if lookupErr != nil {
			return nil, lookupErr
		}
		return nil, fmt.Errorf("upstream host does not resolve to a public address")
	}
	// Some local DNS proxies return RFC 2544 benchmark addresses and expect
	// applications to use the proxy itself. The gateway deliberately bypasses
	// arbitrary proxies, so resolve that hostname through Cloudflare DoH instead.
	addresses, dohErr := resolveWithDoH(ctx, host)
	if dohErr != nil {
		if lookupErr != nil {
			return nil, fmt.Errorf("system DNS: %v; public DNS: %w", lookupErr, dohErr)
		}
		return nil, dohErr
	}
	return addresses, nil
}

func resolveWithDoH(ctx context.Context, host string) ([]net.IP, error) {
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
				ServerName: "cloudflare-dns.com",
			},
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 5 * time.Second,
		},
		Timeout: 8 * time.Second,
	}
	type lookupResult struct {
		index     int
		addresses []net.IP
		err       error
	}
	recordTypes := []int{1, 28}
	results := make(chan lookupResult, len(recordTypes))
	for index, recordType := range recordTypes {
		go func() {
			addresses, err := resolveDoHRecord(ctx, client, host, recordType)
			results <- lookupResult{index: index, addresses: addresses, err: err}
		}()
	}
	ordered := make([][]net.IP, len(recordTypes))
	for range recordTypes {
		lookup := <-results
		if lookup.err != nil {
			return nil, lookup.err
		}
		ordered[lookup.index] = lookup.addresses
	}
	addresses := make([]net.IP, 0, 2)
	for _, records := range ordered {
		addresses = append(addresses, records...)
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("public DNS returned no public address")
	}
	return addresses, nil
}

func resolveDoHRecord(ctx context.Context, client *http.Client, host string, recordType int) ([]net.IP, error) {
	endpoint := "https://1.1.1.1/dns-query?name=" + url.QueryEscape(host) + "&type=" + strconv.Itoa(recordType)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Host = "cloudflare-dns.com"
	request.Header.Set("Accept", "application/dns-json")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Answer []struct {
			Type int    `json:"type"`
			Data string `json:"data"`
		} `json:"Answer"`
	}
	decodeErr := json.NewDecoder(response.Body).Decode(&payload)
	_ = response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("public DNS returned HTTP %d", response.StatusCode)
	}
	if decodeErr != nil {
		return nil, fmt.Errorf("decode public DNS response: %w", decodeErr)
	}
	addresses := make([]net.IP, 0, len(payload.Answer))
	for _, answer := range payload.Answer {
		ip := net.ParseIP(answer.Data)
		if ip != nil && answer.Type == recordType && !UnsafeIP(ip) {
			addresses = append(addresses, ip)
		}
	}
	return addresses, nil
}
