// Package httpclient provides a shared HTTP client with DNS-over-HTTPS
// fallback for commonly polluted Steam domains, optional user-configured
// proxy, connection pooling, and exponential-backoff retries. It exists to
// make external calls reliable from mainland China network environments.
package httpclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Client wraps *http.Client with a mutable proxy/transport so the proxy URL
// can be updated at runtime (e.g. after the user saves settings) without
// handing new client instances to every caller.
type Client struct {
	mu       sync.RWMutex
	proxyURL string
	http     *http.Client
}

var shared = &Client{}

func init() {
	shared.rebuild()
}

// Shared returns the process-wide HTTP client.
func Shared() *Client { return shared }

// SetProxy updates the proxy URL. Empty string means "no proxy" (but the
// underlying transport will still honor HTTP_PROXY / HTTPS_PROXY env vars).
// A trimmed value like "http://127.0.0.1:7890" or "socks5://..." is accepted.
func (c *Client) SetProxy(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw != "" {
		if _, err := url.Parse(raw); err != nil {
			return fmt.Errorf("invalid proxy url: %w", err)
		}
	}
	c.mu.Lock()
	c.proxyURL = raw
	c.mu.Unlock()
	c.rebuild()
	return nil
}

// Proxy returns the currently configured proxy URL.
func (c *Client) Proxy() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.proxyURL
}

func (c *Client) rebuild() {
	c.mu.RLock()
	proxyRaw := c.proxyURL
	c.mu.RUnlock()

	proxyFn := http.ProxyFromEnvironment
	if proxyRaw != "" {
		if u, err := url.Parse(proxyRaw); err == nil {
			proxyFn = http.ProxyURL(u)
		}
	}

	tr := &http.Transport{
		Proxy:                 proxyFn,
		DialContext:           dohDialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          64,
		MaxIdleConnsPerHost:   8,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   8 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	c.mu.Lock()
	c.http = &http.Client{
		Transport: tr,
		Timeout:   60 * time.Second,
	}
	c.mu.Unlock()
}

func (c *Client) httpClient() *http.Client {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.http
}

// Do executes a request with retries. Caller is responsible for closing the
// response body. Retries are attempted on transient errors (network errors,
// 5xx, 429); request bodies are not re-seekable and so non-GET methods with
// a body will only be attempted once.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	const maxAttempts = 3
	backoffs := []time.Duration{0, 300 * time.Millisecond, 900 * time.Millisecond}

	retriable := req.Method == http.MethodGet || req.Method == http.MethodHead || req.Body == nil

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if d := backoffs[attempt]; d > 0 {
			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-time.After(d):
			}
		}
		resp, err := c.httpClient().Do(req)
		if err != nil {
			lastErr = err
			if !retriable || !isTransient(err) {
				return nil, err
			}
			continue
		}
		if retriable && (resp.StatusCode == 429 || resp.StatusCode >= 500) {
			resp.Body.Close()
			lastErr = fmt.Errorf("http status %d", resp.StatusCode)
			continue
		}
		return resp, nil
	}
	return nil, lastErr
}

// Get is a convenience wrapper around Do.
func (c *Client) Get(urlStr string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// GetCtx is Get with an explicit context.
func (c *Client) GetCtx(ctx context.Context, urlStr string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// isTransient classifies an error as retriable.
func isTransient(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	msg := err.Error()
	for _, s := range []string{"reset", "refused", "EOF", "timeout", "broken pipe", "no such host", "unreachable"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// --- Dialer with DoH fallback ---

var baseDialer = &net.Dialer{
	Timeout:   8 * time.Second,
	KeepAlive: 30 * time.Second,
	DualStack: true,
}

// dohDialContext attempts a normal system-resolved dial first. If it fails
// with a DNS-looking error and the host matches the DoH suffix list, it
// re-resolves via DoH and retries each returned IP in turn.
func dohDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	// If addr is already an IP, skip DoH entirely.
	if ip := net.ParseIP(host); ip != nil {
		return baseDialer.DialContext(ctx, network, addr)
	}

	if !shouldUseDoH(host) {
		return baseDialer.DialContext(ctx, network, addr)
	}

	// Fast path: try cached DoH result first (cheap, already-verified)
	if ips := cachedLookup(host); len(ips) > 0 {
		if conn, err := dialIPs(ctx, network, ips, port); err == nil {
			return conn, nil
		}
	}

	// DoH is authoritative for this host: wait briefly for a clean answer and
	// prefer it over the (often poisoned) system resolver. Only if DoH yields
	// nothing dialable do we fall back to the system DNS result.
	ips, dohErr := resolveDoH(ctx, host)
	if dohErr == nil && len(ips) > 0 {
		if conn, err := dialIPs(ctx, network, ips, port); err == nil {
			return conn, nil
		}
	}

	if dohErr == nil {
		dohErr = fmt.Errorf("doh: none of %d ips dialable", len(ips))
	}
	conn, err := baseDialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: doh=%v sys=%v", host, dohErr, err)
	}
	return conn, nil
}

func dialIPs(ctx context.Context, network string, ips []string, port string) (net.Conn, error) {
	var lastErr error
	for _, ip := range ips {
		conn, err := baseDialer.DialContext(ctx, network, net.JoinHostPort(ip, port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no ips to dial")
	}
	return nil, lastErr
}
