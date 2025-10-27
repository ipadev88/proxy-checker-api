package checker

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/net/proxy"
)

// CheckSOCKS4 checks a SOCKS4 proxy (optimized)
func (c *Checker) CheckSOCKS4(ctx context.Context, proxyAddr string, startTime time.Time) CheckResult {
	// Parse proxy address
	dialer, err := proxy.SOCKS5("tcp", proxyAddr, nil, proxy.Direct)
	if err != nil {
		return CheckResult{
			Proxy:    proxyAddr,
			Protocol: "socks4",
			Alive:    false,
			Error:    fmt.Sprintf("SOCKS4 dialer error: %v", err),
		}
	}

	// Simple TCP connection test (much faster than HTTP)
	conn, err := dialer.Dial("tcp", "www.google.com:80")
	if err != nil {
		return CheckResult{
			Proxy:    proxyAddr,
			Protocol: "socks4",
			Alive:    false,
			Error:    fmt.Sprintf("SOCKS4 TCP connection error: %v", err),
		}
	}
	defer conn.Close()

	// If TCP connection succeeds, assume proxy is working
	latency := time.Since(startTime)
	return CheckResult{
		Proxy:     proxyAddr,
		Protocol:  "socks4",
		Alive:     true,
		LatencyMs: latency.Milliseconds(),
	}
}

// CheckSOCKS5 checks a SOCKS5 proxy (optimized)
func (c *Checker) CheckSOCKS5(ctx context.Context, proxyAddr string, startTime time.Time) CheckResult {
	// Create SOCKS5 dialer with no authentication
	dialer, err := proxy.SOCKS5("tcp", proxyAddr, nil, proxy.Direct)
	if err != nil {
		return CheckResult{
			Proxy:    proxyAddr,
			Protocol: "socks5",
			Alive:    false,
			Error:    fmt.Sprintf("SOCKS5 dialer error: %v", err),
		}
	}

	// Simple TCP connection test (much faster than HTTP)
	conn, err := dialer.Dial("tcp", "www.google.com:80")
	if err != nil {
		return CheckResult{
			Proxy:    proxyAddr,
			Protocol: "socks5",
			Alive:    false,
			Error:    fmt.Sprintf("SOCKS5 TCP connection error: %v", err),
		}
	}
	defer conn.Close()

	// If TCP connection succeeds, assume proxy is working
	latency := time.Since(startTime)
	return CheckResult{
		Proxy:     proxyAddr,
		Protocol:  "socks5",
		Alive:     true,
		LatencyMs: latency.Milliseconds(),
	}
}

