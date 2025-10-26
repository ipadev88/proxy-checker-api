package main

import (
	"context"
	"os"
	"os/signal"
	"runtime"
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
	
	// Wait for zmap to finish
	<-zmapDone
	
	// PHASE 4: Check zmap candidates immediately as they arrive
	var zmapResults []checker.CheckResult
	if len(zmapProxies) > 0 {
		log.Infof("Starting immediate check of %d zmap candidates...", len(zmapProxies))
		zmapResults = checkProxiesInBatches(ctx, zmapProxies, chk)
	}
	
	// Merge results
	allResults := append(scrapedResults, zmapResults...)
	
	log.Infof("Total candidates checked: scraped=%d, zmap=%d",
		len(scrapedResults), len(zmapResults))

	// PHASE 5: Process results

	aliveCount := 0
	deadCount := 0
	aliveProxies := make([]snapshot.Proxy, 0)

	for _, result := range allResults {
		if result.Alive {
			aliveCount++
			aliveProxies = append(aliveProxies, snapshot.Proxy{
				Address:   result.Proxy,
				Protocol:  "http", // Determined during check
				Alive:     true,
				LatencyMs: result.LatencyMs,
				LastCheck: time.Now(),
			})
		} else {
			deadCount++
		}
	}

	totalChecked := len(allResults)
	alivePercent := 0.0
	if totalChecked > 0 {
		alivePercent = float64(aliveCount) / float64(totalChecked) * 100.0
	}

	log.Infof("Check complete: %d alive, %d dead (%.2f%% alive)",
		aliveCount, deadCount, alivePercent)

	// Update snapshot
	stats := snapshot.Stats{
		TotalScraped:  totalScraped,
		TotalAlive:    aliveCount,
		TotalDead:     deadCount,
		AlivePercent:  alivePercent,
		LastCheckTime: time.Now(),
		SourceStats:   sourceStats,
	}

	snap.Update(aliveProxies, stats)

	totalDuration := time.Since(start)
	log.Infof("Aggregation cycle complete in %v", totalDuration)

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
	
	// Full check
	log.Infof("Full checking %d proxies...", len(proxies))
	results := make([]checker.CheckResult, len(proxies))
	for i, p := range proxies {
		results[i] = chk.CheckProxyWithProtocol(ctx, p.Address, p.Protocol)
	}
	
	return results
}

