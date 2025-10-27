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
	"syscall"
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
	Protocol  string // "http", "socks4", "socks5"
	Alive     bool
	LatencyMs int64
	Error     string
}

func NewChecker(cfg config.CheckerConfig, metricsCollector *metrics.Collector) *Checker {
	// Create highly optimized transport for mass concurrency
	transport := &http.Transport{
		Proxy: nil, // We set proxy per-request
		DialContext: (&net.Dialer{
			Timeout:   time.Duration(cfg.TimeoutMs/2) * time.Millisecond, // Faster dial
			KeepAlive: 15 * time.Second, // Shorter keep-alive for proxy checking
		}).DialContext,
		ForceAttemptHTTP2:     false, // Disable HTTP/2 for proxy checking
		MaxIdleConns:          cfg.ConcurrencyTotal / 10, // Reduced idle connections
		MaxIdleConnsPerHost:   10, // Much lower per-host limit
		MaxConnsPerHost:       0, // No limit
		IdleConnTimeout:       30 * time.Second, // Shorter idle timeout
		TLSHandshakeTimeout:   time.Duration(cfg.TimeoutMs/2) * time.Millisecond,
		ExpectContinueTimeout: 500 * time.Millisecond, // Faster expect timeout
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

// GetConfig returns the checker configuration
func (c *Checker) GetConfig() *config.CheckerConfig {
	return &c.config
}

// CheckProxies performs high-concurrency proxy validation
func (c *Checker) CheckProxies(ctx context.Context, proxies []string) []CheckResult {
	totalProxies := len(proxies)

	// Adaptive concurrency adjustment
	adaptiveConcurrency := c.config.ConcurrencyTotal
	if c.config.EnableAdaptiveConcurrency {
		adaptiveConcurrency = c.adjustConcurrency(adaptiveConcurrency)
	}

	log.Infof("Starting proxy check: %d proxies, concurrency=%d (adaptive), batch_size=%d (adaptive)",
		totalProxies, adaptiveConcurrency, c.config.BatchSize)

	results := make([]CheckResult, 0, totalProxies)
	resultsMu := sync.Mutex{}

	// Semaphore for concurrency control
	sem := make(chan struct{}, adaptiveConcurrency)

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

	// Adaptive batch sizing based on system resources
	adaptiveBatchSize := c.config.BatchSize
	if adaptiveBatchSize <= 0 {
		adaptiveBatchSize = 1000 // Default smaller batch
	}

	// Reduce batch size if high concurrency to avoid memory spikes
	if adaptiveConcurrency > 5000 {
		adaptiveBatchSize = adaptiveBatchSize / 2
	}

	// Process in adaptive batches
	var wg sync.WaitGroup

	for i := 0; i < totalProxies; i += adaptiveBatchSize {
		end := i + adaptiveBatchSize
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
				result.Protocol = "http" // CheckProxies always checks HTTP proxies

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
		if i+adaptiveBatchSize < totalProxies {
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
		adjusted := requested * 6 / 10 // Reduce by 40% when high load
		log.Warnf("High goroutine count (%d), reducing concurrency: %d -> %d",
			numGoroutines, requested, adjusted)
		return adjusted
	}

	// Check file descriptor usage
	var rlim syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlim); err == nil {
		usedFDs := float64(requested) * 1.5 // Estimate FDs needed
		availableFDs := float64(rlim.Cur) * float64(c.config.MaxFdUsagePercent) / 100.0

		if usedFDs > availableFDs {
			adjusted := int(availableFDs / 1.5)
			if adjusted < 100 {
				adjusted = 100 // Minimum
			}
			log.Warnf("High FD usage (limit: %d, needed: %.0f), reducing concurrency: %d -> %d",
				rlim.Cur, usedFDs, requested, adjusted)
			return adjusted
		}
	}

	// Check memory usage
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	memUsageGB := float64(m.Alloc) / 1024 / 1024 / 1024
	maxMemGB := 2.0 // 2GB limit

	if memUsageGB > maxMemGB {
		adjusted := requested * 7 / 10 // Reduce by 30%
		log.Warnf("High memory usage (%.2fGB), reducing concurrency: %d -> %d",
			memUsageGB, requested, adjusted)
		return adjusted
	}

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
					Protocol:  "socks4",
					Alive:     false,
					LatencyMs: 0,
					Error:     "SOCKS checking disabled",
				}
			}
			result = c.CheckSOCKS4(ctx, proxyAddr, startTime)
			result.Protocol = "socks4"
		} else if protocol == "socks5" {
			if !c.config.SocksEnabled {
				return CheckResult{
					Proxy:     proxyAddr,
					Protocol:  "socks5",
					Alive:     false,
					LatencyMs: 0,
					Error:     "SOCKS checking disabled",
				}
			}
			result = c.CheckSOCKS5(ctx, proxyAddr, startTime)
			result.Protocol = "socks5"
		} else {
			// Use HTTP checker
			result = c.checkProxyWithRetries(ctx, proxyAddr)
			result.Protocol = "http"
		}

		if result.Alive {
			return result
		}

		lastError = result.Error
	}

	return CheckResult{
		Proxy:    proxyAddr,
		Protocol: protocol,
		Alive:    false,
		Error:    lastError,
	}
}

// CheckProxyWithProtocol is an alias for CheckSingleWithProtocol
func (c *Checker) CheckProxyWithProtocol(ctx context.Context, address string, protocol string) CheckResult {
	return c.CheckSingleWithProtocol(ctx, address, protocol)
}

