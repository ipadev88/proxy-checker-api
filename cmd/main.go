package main

import (
	"context"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/proxy-checker-api/internal/aggregator"
	"github.com/proxy-checker-api/internal/api"
	"github.com/proxy-checker-api/internal/checker"
	"github.com/proxy-checker-api/internal/config"
	"github.com/proxy-checker-api/internal/metrics"
	"github.com/proxy-checker-api/internal/snapshot"
	"github.com/proxy-checker-api/internal/storage"
	"github.com/proxy-checker-api/internal/zmap"
	log "github.com/sirupsen/logrus"
)

const version = "1.0.0"

func main() {
	log.SetFormatter(&log.JSONFormatter{})
	log.SetLevel(log.InfoLevel)
	log.Infof("Starting Proxy Checker Service v%s", version)

	// Load configuration
	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Set log level
	if level, err := log.ParseLevel(cfg.Logging.Level); err == nil {
		log.SetLevel(level)
	}

	// Set GOMAXPROCS to use all available CPUs
	numCPU := runtime.NumCPU()
	runtime.GOMAXPROCS(numCPU)
	log.Infof("GOMAXPROCS set to %d", numCPU)

	// Initialize metrics
	metricsCollector := metrics.NewCollector(cfg.Metrics.Namespace)

	// Initialize storage
	store, err := storage.NewStorage(cfg.Storage.Type, cfg.Storage.Path)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer store.Close()

	// Initialize snapshot manager
	snapshotMgr := snapshot.NewManager(store, cfg.Storage.PersistIntervalSeconds)

	// Load existing proxies from storage
	if err := snapshotMgr.LoadFromStorage(); err != nil {
		log.Warnf("Failed to load existing snapshot: %v (starting fresh)", err)
	}

	// Initialize aggregator
	agg := aggregator.NewAggregator(cfg.Aggregator, metricsCollector)

	// Initialize checker
	chk := checker.NewChecker(cfg.Checker, metricsCollector)

	// Initialize zmap scanner (if enabled)
	var zmapScanner *zmap.ZmapScanner
	if cfg.Zmap.Enabled {
		log.Info("Zmap scanning is enabled")
		
		// Verify zmap setup
		if err := zmap.VerifyZmapSetup(cfg.Zmap); err != nil {
			log.Warnf("Zmap setup verification failed: %v", err)
			log.Warn("Zmap scanning will be disabled")
			cfg.Zmap.Enabled = false
		} else {
			zmapScanner = zmap.NewZmapScanner(cfg.Zmap, metricsCollector)
			log.Infof("Zmap scanner initialized for ports: %v", cfg.Zmap.Ports)
		}
	} else {
		log.Info("Zmap scanning is disabled")
	}

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start aggregation loop
	go runAggregationLoop(ctx, agg, chk, snapshotMgr, zmapScanner, &cfg.Checker, cfg.Aggregator.IntervalSeconds)

	// Start API server
	apiServer := api.NewServer(cfg, snapshotMgr, metricsCollector, agg, chk, zmapScanner)
	go func() {
		if err := apiServer.Start(); err != nil {
			log.Fatalf("API server failed: %v", err)
		}
	}()

	log.Infof("Service started successfully on %s", cfg.API.Addr)

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Info("Shutting down gracefully...")
	cancel()

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := apiServer.Shutdown(shutdownCtx); err != nil {
		log.Errorf("API server shutdown error: %v", err)
	}

	log.Info("Shutdown complete")
}

func runAggregationLoop(ctx context.Context, agg *aggregator.Aggregator, chk *checker.Checker, snap *snapshot.Manager, zmapScanner *zmap.ZmapScanner, checkerCfg *config.CheckerConfig, intervalSeconds int) {
	// Run immediately on startup
	runAggregationCycle(ctx, agg, chk, snap, zmapScanner, checkerCfg)

	ticker := time.NewTicker(time.Duration(intervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("Aggregation loop stopped")
			return
		case <-ticker.C:
			runAggregationCycle(ctx, agg, chk, snap, zmapScanner, checkerCfg)
		}
	}
}

func runAggregationCycle(ctx context.Context, agg *aggregator.Aggregator, chk *checker.Checker, snap *snapshot.Manager, zmapScanner *zmap.ZmapScanner, checkerCfg *config.CheckerConfig) {
	start := time.Now()
	log.Info("Starting aggregation cycle")

	// PHASE 1: Fetch proxies from HTTP sources
	scrapedProxies, sourceStats, err := agg.Aggregate(ctx)
	if err != nil {
		log.Errorf("Aggregation failed: %v", err)
		return
	}

	totalScraped := len(scrapedProxies)
	log.Infof("Aggregated %d unique proxies from %d sources", totalScraped, len(sourceStats))

	// PHASE 2: Run zmap scan in parallel with checking
	var zmapProxies []aggregator.ProxyWithProtocol
	zmapDone := make(chan bool)
	
	if zmapScanner != nil {
		log.Info("Running zmap scan in parallel...")
		go func() {
			var err error
			zmapProxies, err = zmapScanner.ScanWithProtocol(ctx)
			if err != nil {
				log.Errorf("Zmap scan failed: %v", err)
			} else {
				log.Infof("Zmap scan found %d candidates", len(zmapProxies))
			}
			zmapDone <- true
		}()
	} else {
		zmapDone <- true // Skip if zmap disabled
	}

	// PHASE 3: Start checking scraped proxies immediately
	log.Info("Starting immediate check of scraped proxies...")
	scrapedResults := checkProxiesInBatches(ctx, scrapedProxies, chk)
	
	// PHASE 4: Process scraped results IMMEDIATELY and update snapshot
	log.Info("Processing scraped proxy results...")
	
	scrapedAliveCount := 0
	scrapedDeadCount := 0
	scrapedAliveProxies := make([]snapshot.Proxy, 0)

	for _, result := range scrapedResults {
		if result.Alive {
			scrapedAliveCount++
			scrapedAliveProxies = append(scrapedAliveProxies, snapshot.Proxy{
				Address:   result.Proxy,
				Protocol:  result.Protocol,
				Source:    "scraped",
				Alive:     true,
				LatencyMs: result.LatencyMs,
				LastCheck: time.Now(),
			})
		} else {
			scrapedDeadCount++
		}
	}

	scrapedTotalChecked := len(scrapedResults)
	scrapedAlivePercent := 0.0
	if scrapedTotalChecked > 0 {
		scrapedAlivePercent = float64(scrapedAliveCount) / float64(scrapedTotalChecked) * 100.0
	}

	log.Infof("Scraped proxies: %d alive, %d dead (%.2f%% alive)",
		scrapedAliveCount, scrapedDeadCount, scrapedAlivePercent)

	// Update snapshot with scraped results IMMEDIATELY (don't wait for zmap)
	scrapedStats := snapshot.Stats{
		TotalScraped:  totalScraped,
		TotalAlive:    scrapedAliveCount,
		TotalDead:     scrapedDeadCount,
		AlivePercent:  scrapedAlivePercent,
		LastCheckTime: time.Now(),
		SourceStats:   sourceStats,
	}
	
	snap.Update(scrapedAliveProxies, scrapedStats)
	log.Info("Snapshot updated with scraped proxies (zmap running in background)")

	// PHASE 5: Wait for zmap in background and process when ready
	go func() {
		// Wait for zmap to finish (with reasonable timeout)
		select {
		case <-zmapDone:
			log.Info("Zmap scan completed, processing candidates...")
		case <-time.After(15 * time.Minute):
			log.Warn("Zmap timeout exceeded 15 minutes")
			return
		}
		
		// Check zmap candidates if available
		if len(zmapProxies) == 0 {
			log.Info("No zmap candidates to check")
			return
		}

		log.Infof("Starting check of %d zmap candidates...", len(zmapProxies))
		zmapResults := checkProxiesInBatches(ctx, zmapProxies, chk)
		
		// Process zmap results
		zmapAliveProxies := make([]snapshot.Proxy, 0)
		zmapAliveCount := 0
		zmapDeadCount := 0
		
		for _, result := range zmapResults {
			if result.Alive {
				zmapAliveCount++
				zmapAliveProxies = append(zmapAliveProxies, snapshot.Proxy{
					Address:   result.Proxy,
					Protocol:  result.Protocol,
					Source:    "zmap",
					Alive:     true,
					LatencyMs: result.LatencyMs,
					LastCheck: time.Now(),
				})
			} else {
				zmapDeadCount++
			}
		}

		log.Infof("Zmap proxies: %d alive, %d dead", zmapAliveCount, zmapDeadCount)

		// Merge zmap results with existing scraped proxies
		allAliveProxies := append(scrapedAliveProxies, zmapAliveProxies...)
		totalAlive := scrapedAliveCount + zmapAliveCount
		totalDead := scrapedDeadCount + zmapDeadCount
		totalAll := totalAlive + totalDead
		
		overallAlivePercent := 0.0
		if totalAll > 0 {
			overallAlivePercent = float64(totalAlive) / float64(totalAll) * 100.0
		}

		// Update snapshot with combined results
		combinedStats := snapshot.Stats{
			TotalScraped:  totalScraped,
			TotalAlive:    totalAlive,
			TotalDead:     totalDead,
			AlivePercent:  overallAlivePercent,
			LastCheckTime: time.Now(),
			SourceStats:   sourceStats,
		}
		
		snap.Update(allAliveProxies, combinedStats)
		log.Infof("Snapshot updated with zmap results: total %d alive (scraped: %d, zmap: %d)",
			totalAlive, scrapedAliveCount, zmapAliveCount)
	}()

	log.Infof("Aggregation cycle complete for scraped proxies: %d alive, %d dead (%.2f%% alive)",
		scrapedAliveCount, scrapedDeadCount, scrapedAlivePercent)
	log.Info("Zmap scan continues in background...")

	totalDuration := time.Since(start)
	log.Infof("Aggregation cycle completed in %v", totalDuration)

	// Log memory stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	log.Infof("Memory: Alloc=%dMB, TotalAlloc=%dMB, Sys=%dMB, NumGC=%d, Goroutines=%d",
		m.Alloc/1024/1024, m.TotalAlloc/1024/1024, m.Sys/1024/1024, m.NumGC, runtime.NumGoroutine())
}

// deduplicateProxiesWithProtocol removes duplicate proxy addresses with protocol awareness
func deduplicateProxiesWithProtocol(proxies []aggregator.ProxyWithProtocol) []aggregator.ProxyWithProtocol {
	seen := make(map[string]struct{}, len(proxies))
	unique := make([]aggregator.ProxyWithProtocol, 0, len(proxies))

	for _, proxy := range proxies {
		// Key by address + protocol
		key := proxy.Address + "|" + proxy.Protocol
		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			unique = append(unique, proxy)
		}
	}

	return unique
}

// checkProxiesInBatches checks proxies with fast filter and full check
func checkProxiesInBatches(ctx context.Context, proxies []aggregator.ProxyWithProtocol, chk *checker.Checker) []checker.CheckResult {
	if len(proxies) == 0 {
		return []checker.CheckResult{}
	}
	
	cfg := chk.GetConfig()
	
	// Fast filter if enabled
	addresses := make([]string, len(proxies))
	for i, p := range proxies {
		addresses[i] = p.Address
	}
	
	if cfg.EnableFastFilter && len(addresses) > 1000 {
		log.Infof("Fast filtering %d proxies...", len(addresses))
		filtered := checker.FastConnectFilter(ctx, addresses, cfg.FastFilterTimeoutMs, cfg.FastFilterConcurrency)
		
		// Keep only filtered
		filteredMap := make(map[string]bool)
		for _, addr := range filtered {
			filteredMap[addr] = true
		}
		
		var filteredProxies []aggregator.ProxyWithProtocol
		for _, p := range proxies {
			if filteredMap[p.Address] {
				filteredProxies = append(filteredProxies, p)
			}
		}
		proxies = filteredProxies
		log.Infof("Fast filter: %d/%d passed", len(proxies), len(addresses))
	}
	
	// Full check with concurrency
	log.Infof("Full checking %d proxies with protocol awareness...", len(proxies))
	
	// First group by protocol for batch checking
	httpProxies := make([]string, 0)
	socks4Proxies := make([]string, 0)
	socks5Proxies := make([]string, 0)
	indexMap := make(map[string]int) // address -> original index
	
	for i, p := range proxies {
		indexMap[p.Address] = i
		switch p.Protocol {
		case "socks4":
			socks4Proxies = append(socks4Proxies, p.Address)
		case "socks5":
			socks5Proxies = append(socks5Proxies, p.Address)
		default:
			httpProxies = append(httpProxies, p.Address)
		}
	}
	
	results := make([]checker.CheckResult, len(proxies))
	
	// Check HTTP proxies in parallel
	if len(httpProxies) > 0 {
		log.Infof("Checking %d HTTP proxies...", len(httpProxies))
		httpResults := chk.CheckProxies(ctx, httpProxies)
		for _, res := range httpResults {
			if idx, ok := indexMap[res.Proxy]; ok {
				results[idx] = res
			}
		}
	}
	
	// Check SOCKS4 proxies in parallel
	if len(socks4Proxies) > 0 {
		log.Infof("Checking %d SOCKS4 proxies...", len(socks4Proxies))
		var wg sync.WaitGroup
		var mu sync.Mutex
		concurrency := 1000 // Limit concurrent SOCKS checks
		sem := make(chan struct{}, concurrency)
		
		for _, addr := range socks4Proxies {
			wg.Add(1)
			go func(address string) {
				defer wg.Done()
				sem <- struct{}{}        // Acquire
				defer func() { <-sem }() // Release
				
				result := chk.CheckProxyWithProtocol(ctx, address, "socks4")
				mu.Lock()
				if idx, ok := indexMap[address]; ok {
					results[idx] = result
				}
				mu.Unlock()
			}(addr)
		}
		wg.Wait()
	}
	
	// Check SOCKS5 proxies in parallel
	if len(socks5Proxies) > 0 {
		log.Infof("Checking %d SOCKS5 proxies...", len(socks5Proxies))
		var wg sync.WaitGroup
		var mu sync.Mutex
		concurrency := 1000 // Limit concurrent SOCKS checks
		sem := make(chan struct{}, concurrency)
		
		for _, addr := range socks5Proxies {
			wg.Add(1)
			go func(address string) {
				defer wg.Done()
				sem <- struct{}{}        // Acquire
				defer func() { <-sem }() // Release
				
				result := chk.CheckProxyWithProtocol(ctx, address, "socks5")
				mu.Lock()
				if idx, ok := indexMap[address]; ok {
					results[idx] = result
				}
				mu.Unlock()
			}(addr)
		}
		wg.Wait()
	}
	
	log.Infof("Full check complete: processed %d proxies", len(results))
	return results
}

