package checker

import (
	"context"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"
)

// FastConnectFilter performs TCP-only connection pre-filtering
// This quickly filters out dead proxies before running full HTTP checks
func FastConnectFilter(ctx context.Context, proxies []string, timeoutMs int, concurrency int) []string {
	if len(proxies) == 0 {
		return proxies
	}

	log.Infof("Starting fast TCP filter: %d proxies, concurrency=%d, timeout=%dms",
		len(proxies), concurrency, timeoutMs)

	startTime := time.Now()
	timeout := time.Duration(timeoutMs) * time.Millisecond

	connectable := make([]string, 0, len(proxies)/5) // Estimate ~20% alive
	var mu sync.Mutex

	// Semaphore for concurrency control
	sem := make(chan struct{}, concurrency)

	// Progress tracking
	var completed atomic.Int64
	var successful atomic.Int64
	progressTicker := time.NewTicker(5 * time.Second)
	defer progressTicker.Stop()

	go func() {
		for range progressTicker.C {
			current := completed.Load()
			success := successful.Load()
			percent := float64(current) / float64(len(proxies)) * 100.0
			log.Infof("Fast filter progress: %d/%d (%.1f%%), connectable=%d, goroutines=%d",
				current, len(proxies), percent, success, runtime.NumGoroutine())
		}
	}()

	var wg sync.WaitGroup

	// Test all proxies
	for _, proxy := range proxies {
		sem <- struct{}{} // Acquire semaphore
		wg.Add(1)

		go func(proxyAddr string) {
			defer wg.Done()
			defer func() { <-sem }() // Release semaphore

			// TCP connect test
			if testTCPConnection(proxyAddr, timeout) {
				mu.Lock()
				connectable = append(connectable, proxyAddr)
				mu.Unlock()
				successful.Add(1)
			}

			completed.Add(1)
		}(proxy)
	}

	wg.Wait()

	duration := time.Since(startTime)
	filteredOut := len(proxies) - len(connectable)
	filterRate := float64(filteredOut) / float64(len(proxies)) * 100.0

	log.Infof("Fast filter complete: %d/%d connectable (%.1f%% filtered out) in %v (%.0f tests/sec)",
		len(connectable), len(proxies), filterRate, duration, float64(len(proxies))/duration.Seconds())

	return connectable
}

// testTCPConnection tests if a TCP connection can be established
func testTCPConnection(address string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

