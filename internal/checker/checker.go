package checker

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/proxy-checker-api/internal/config"
	"github.com/proxy-checker-api/internal/metrics"
	log "github.com/sirupsen/logrus"
)

type Checker struct {
	config    config.CheckerConfig
	metrics   *metrics.Collector
	transport *http.Transport
	client    *http.Client
}

type CheckResult struct {
	Proxy     string
	Alive     bool
	LatencyMs int64
	Error     string
}

func NewChecker(cfg config.CheckerConfig, metricsCollector *metrics.Collector) *Checker {
	// Create highly optimized transport for mass concurrency
	transport := &http.Transport{
		Proxy: nil, // We set proxy per-request
		DialContext: (&net.Dialer{
			Timeout:   time.Duration(cfg.TimeoutMs) * time.Millisecond,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     false, // Disable HTTP/2 for proxy checking
		MaxIdleConns:          cfg.ConcurrencyTotal,
		MaxIdleConnsPerHost:   100,
		MaxConnsPerHost:       0, // No limit
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   time.Duration(cfg.TimeoutMs) * time.Millisecond,
		ExpectContinueTimeout: 1 * time.Second,
		DisableKeepAlives:     false,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // Required for proxy checking
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   time.Duration(cfg.TimeoutMs) * time.Millisecond,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Don't follow redirects
		},
	}

	return &Checker{
		config:    cfg,
		metrics:   metricsCollector,
		transport: transport,
		client:    client,
	}
}

// CheckProxies performs high-concurrency proxy validation
func (c *Checker) CheckProxies(ctx context.Context, proxies []string) []CheckResult {
	totalProxies := len(proxies)
	log.Infof("Starting proxy check: %d proxies, concurrency=%d", totalProxies, c.config.ConcurrencyTotal)

	startTime := time.Now()

	// Adaptive concurrency adjustment
	concurrency := c.config.ConcurrencyTotal
	if c.config.EnableAdaptiveConcurrency {
		concurrency = c.adjustConcurrency(concurrency)
	}

	results := make([]CheckResult, 0, totalProxies)
	resultsMu := sync.Mutex{}

	// Semaphore for concurrency control
	sem := make(chan struct{}, concurrency)

	// Progress tracking
	var completed atomic.Int64
	progressTicker := time.NewTicker(5 * time.Second)
	defer progressTicker.Stop()

	go func() {
		for range progressTicker.C {
			current := completed.Load()
			percent := float64(current) / float64(totalProxies) * 100.0
			log.Infof("Progress: %d/%d (%.1f%%), goroutines=%d",
				current, totalProxies, percent, runtime.NumGoroutine())
		}
	}()

	// Process in batches
	batchSize := c.config.BatchSize
	if batchSize <= 0 {
		batchSize = 2000
	}

	var wg sync.WaitGroup

	for i := 0; i < totalProxies; i += batchSize {
		end := i + batchSize
		if end > totalProxies {
			end = totalProxies
		}

		batch := proxies[i:end]

		for _, proxy := range batch {
			// Acquire semaphore
			sem <- struct{}{}
			wg.Add(1)

			go func(proxyAddr string) {
				defer wg.Done()
				defer func() { <-sem }() // Release semaphore

				// Check with retries
				result := c.checkProxyWithRetries(ctx, proxyAddr)

				resultsMu.Lock()
				results = append(results, result)
				resultsMu.Unlock()

				completed.Add(1)

				// Record metrics
				if result.Alive {
					c.metrics.RecordCheckSuccess()
					c.metrics.RecordCheckDuration(float64(result.LatencyMs) / 1000.0)
				} else {
					c.metrics.RecordCheckFailure()
				}
			}(proxy)
		}

		// Small delay between batches to prevent thundering herd
		if i+batchSize < totalProxies {
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Wait for all checks to complete
	wg.Wait()

	duration := time.Since(startTime)
	checksPerSecond := float64(totalProxies) / duration.Seconds()
	log.Infof("Check complete: %d proxies in %v (%.0f checks/sec)",
		totalProxies, duration, checksPerSecond)

	return results
}

func (c *Checker) checkProxyWithRetries(ctx context.Context, proxyAddr string) CheckResult {
	maxRetries := c.config.Retries
	if maxRetries < 0 {
		maxRetries = 0
	}

	var lastError string

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			backoff := time.Duration(attempt*attempt*100) * time.Millisecond
			time.Sleep(backoff)
		}

		result := c.checkProxy(ctx, proxyAddr)
		if result.Alive {
			return result
		}

		lastError = result.Error
	}

	return CheckResult{
		Proxy: proxyAddr,
		Alive: false,
		Error: lastError,
	}
}

func (c *Checker) checkProxy(ctx context.Context, proxyAddr string) CheckResult {
	startTime := time.Now()

	if c.config.Mode == "connect-only" {
		return c.checkConnectOnly(ctx, proxyAddr, startTime)
	}

	// Full HTTP check
	return c.checkFullHTTP(ctx, proxyAddr, startTime)
}

func (c *Checker) checkConnectOnly(ctx context.Context, proxyAddr string, startTime time.Time) CheckResult {
	timeout := time.Duration(c.config.TimeoutMs) * time.Millisecond
	conn, err := net.DialTimeout("tcp", proxyAddr, timeout)
	if err != nil {
		return CheckResult{
			Proxy: proxyAddr,
			Alive: false,
			Error: fmt.Sprintf("connect: %v", err),
		}
	}
	defer conn.Close()

	latency := time.Since(startTime)
	return CheckResult{
		Proxy:     proxyAddr,
		Alive:     true,
		LatencyMs: latency.Milliseconds(),
	}
}

func (c *Checker) checkFullHTTP(ctx context.Context, proxyAddr string, startTime time.Time) CheckResult {
	proxyURL, err := url.Parse(fmt.Sprintf("http://%s", proxyAddr))
	if err != nil {
		return CheckResult{
			Proxy: proxyAddr,
			Alive: false,
			Error: fmt.Sprintf("parse proxy URL: %v", err),
		}
	}

	// Create request with timeout context
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(c.config.TimeoutMs)*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "GET", c.config.TestURL, nil)
	if err != nil {
		return CheckResult{
			Proxy: proxyAddr,
			Alive: false,
			Error: fmt.Sprintf("create request: %v", err),
		}
	}

	// Set proxy for this request
	c.transport.Proxy = http.ProxyURL(proxyURL)

	resp, err := c.client.Do(req)
	if err != nil {
		return CheckResult{
			Proxy: proxyAddr,
			Alive: false,
			Error: fmt.Sprintf("request: %v", err),
		}
	}
	defer resp.Body.Close()

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

// adjustConcurrency adapts concurrency based on system resources
func (c *Checker) adjustConcurrency(requested int) int {
	// Check goroutine count
	numGoroutines := runtime.NumGoroutine()
	if numGoroutines > requested*2 {
		adjusted := requested * 8 / 10 // Reduce by 20%
		log.Warnf("High goroutine count (%d), reducing concurrency: %d -> %d",
			numGoroutines, requested, adjusted)
		return adjusted
	}

	// Could also check file descriptors, CPU usage, etc.
	// For now, return requested
	return requested
}

// CheckSingle checks a single proxy (used by API for on-demand checks)
func (c *Checker) CheckSingle(ctx context.Context, proxyAddr string) CheckResult {
	return c.checkProxyWithRetries(ctx, proxyAddr)
}

// CheckSingleWithProtocol checks a single proxy with protocol awareness
func (c *Checker) CheckSingleWithProtocol(ctx context.Context, proxyAddr string, protocol string) CheckResult {
	startTime := time.Now()
	
	maxRetries := c.config.Retries
	if maxRetries < 0 {
		maxRetries = 0
	}

	var lastError string

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt*attempt*100) * time.Millisecond
			time.Sleep(backoff)
		}

		var result CheckResult
		if protocol == "socks4" {
			if !c.config.SocksEnabled {
				return CheckResult{
					Proxy:     proxyAddr,
					Alive:     false,
					LatencyMs: 0,
					Error:     "SOCKS checking disabled",
				}
			}
			result = c.CheckSOCKS4(ctx, proxyAddr, startTime)
		} else if protocol == "socks5" {
			if !c.config.SocksEnabled {
				return CheckResult{
					Proxy:     proxyAddr,
					Alive:     false,
					LatencyMs: 0,
					Error:     "SOCKS checking disabled",
				}
			}
			result = c.CheckSOCKS5(ctx, proxyAddr, startTime)
		} else {
			// Use HTTP checker
			result = c.checkProxyWithRetries(ctx, proxyAddr)
		}

		if result.Alive {
			return result
		}

		lastError = result.Error
	}

	return CheckResult{
		Proxy: proxyAddr,
		Alive: false,
		Error: lastError,
	}
}

// CheckProxyWithProtocol is an alias for CheckSingleWithProtocol
func (c *Checker) CheckProxyWithProtocol(ctx context.Context, address string, protocol string) CheckResult {
	return c.CheckSingleWithProtocol(ctx, address, protocol)
}

