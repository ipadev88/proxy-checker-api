package checker

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/proxy"
)

// CheckSOCKS4 checks a SOCKS4 proxy
func (c *Checker) CheckSOCKS4(ctx context.Context, proxyAddr string, startTime time.Time) CheckResult {
	// Parse proxy address
	dialer, err := proxy.SOCKS5("tcp", proxyAddr, nil, proxy.Direct)
	if err != nil {
		return CheckResult{
			Proxy: proxyAddr,
			Alive: false,
			Error: fmt.Sprintf("SOCKS4 dialer error: %v", err),
		}
	}

	// Create HTTP transport with SOCKS4 proxy
	transport := &http.Transport{
		Dial: func(network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		},
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   time.Duration(c.config.SocksTimeoutMs) * time.Millisecond,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Test the proxy
	testURL := c.config.SocksTestURL
	if testURL == "" {
		testURL = "https://www.google.com/generate_204"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", testURL, nil)
	if err != nil {
		return CheckResult{
			Proxy: proxyAddr,
			Alive: false,
			Error: fmt.Sprintf("request creation error: %v", err),
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return CheckResult{
			Proxy: proxyAddr,
			Alive: false,
			Error: fmt.Sprintf("SOCKS4 connection error: %v", err),
		}
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	latency := time.Since(startTime)

	// Consider 2xx and 3xx as success
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return CheckResult{
			Proxy:     proxyAddr,
			Alive:     true,
			LatencyMs: latency.Milliseconds(),
		}
	}

	return CheckResult{
		Proxy: proxyAddr,
		Alive: false,
		Error: fmt.Sprintf("HTTP %d", resp.StatusCode),
	}
}

// CheckSOCKS5 checks a SOCKS5 proxy
func (c *Checker) CheckSOCKS5(ctx context.Context, proxyAddr string, startTime time.Time) CheckResult {
	// Create SOCKS5 dialer with no authentication
	dialer, err := proxy.SOCKS5("tcp", proxyAddr, nil, proxy.Direct)
	if err != nil {
		return CheckResult{
			Proxy: proxyAddr,
			Alive: false,
			Error: fmt.Sprintf("SOCKS5 dialer error: %v", err),
		}
	}

	// Create HTTP transport with SOCKS5 proxy
	transport := &http.Transport{
		Dial: func(network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		},
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   time.Duration(c.config.SocksTimeoutMs) * time.Millisecond,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Test the proxy
	testURL := c.config.SocksTestURL
	if testURL == "" {
		testURL = "https://www.google.com/generate_204"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", testURL, nil)
	if err != nil {
		return CheckResult{
			Proxy: proxyAddr,
			Alive: false,
			Error: fmt.Sprintf("request creation error: %v", err),
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return CheckResult{
			Proxy: proxyAddr,
			Alive: false,
			Error: fmt.Sprintf("SOCKS5 connection error: %v", err),
		}
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	latency := time.Since(startTime)

	// Consider 2xx and 3xx as success
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return CheckResult{
			Proxy:     proxyAddr,
			Alive:     true,
			LatencyMs: latency.Milliseconds(),
		}
	}

	return CheckResult{
		Proxy: proxyAddr,
		Alive: false,
		Error: fmt.Sprintf("HTTP %d", resp.StatusCode),
	}
}

// CheckProxyWithProtocol checks a proxy based on its protocol
func (c *Checker) CheckProxyWithProtocol(ctx context.Context, proxyAddr string, protocol string) CheckResult {
	startTime := time.Now()

	switch protocol {
	case "socks4":
		if !c.config.SocksEnabled {
			return CheckResult{
				Proxy: proxyAddr,
				Alive: false,
				Error: "SOCKS checking is disabled",
			}
		}
		return c.CheckSOCKS4(ctx, proxyAddr, startTime)

	case "socks5":
		if !c.config.SocksEnabled {
			return CheckResult{
				Proxy: proxyAddr,
				Alive: false,
				Error: "SOCKS checking is disabled",
			}
		}
		return c.CheckSOCKS5(ctx, proxyAddr, startTime)

	case "http", "https":
		return c.checkFullHTTP(ctx, proxyAddr, startTime)

	default:
		// Try HTTP by default
		return c.checkFullHTTP(ctx, proxyAddr, startTime)
	}
}

