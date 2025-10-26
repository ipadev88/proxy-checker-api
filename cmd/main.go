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

	// PHASE 2: Run zmap scan (if enabled)
	var zmapProxies []aggregator.ProxyWithProtocol
	if zmapScanner != nil {
		log.Info("Running zmap scan...")
		zmapProxies, err = zmapScanner.ScanWithProtocol(ctx)
		if err != nil {
			log.Errorf("Zmap scan failed: %v", err)
		} else {
			log.Infof("Zmap scan found %d candidates", len(zmapProxies))
		}
	}

	// PHASE 3: Merge and deduplicate
	allProxies := append(scrapedProxies, zmapProxies...)
	proxies := deduplicateProxiesWithProtocol(allProxies)
	
	log.Infof("Total unique proxies after merge: %d (scraped=%d, zmap=%d)",
		len(proxies), len(scrapedProxies), len(zmapProxies))

	if len(proxies) == 0 {
		log.Warn("No proxies to check, skipping check cycle")
		return
	}

	// PHASE 4: Fast TCP filter (if enabled)
	if checkerCfg.EnableFastFilter && len(proxies) > 1000 {
		log.Info("Running fast TCP filter...")
		filterStart := time.Now()
		
		// Extract addresses for fast filter
		addresses := make([]string, len(proxies))
		for i, p := range proxies {
			addresses[i] = p.Address
		}
		
		// Filter
		filteredAddresses := checker.FastConnectFilter(ctx, addresses, checkerCfg.FastFilterTimeoutMs, checkerCfg.FastFilterConcurrency)
		filterDuration := time.Since(filterStart)
		
		// Create map of filtered addresses
		filteredMap := make(map[string]bool)
		for _, addr := range filteredAddresses {
			filteredMap[addr] = true
		}
		
		// Keep only proxies that passed filter
		var filteredProxies []aggregator.ProxyWithProtocol
		for _, p := range proxies {
			if filteredMap[p.Address] {
				filteredProxies = append(filteredProxies, p)
			}
		}
		proxies = filteredProxies
		
		log.Infof("Fast filter complete: %d connectable proxies in %v", len(proxies), filterDuration)
		
		if len(proxies) == 0 {
			log.Warn("No proxies passed fast filter, skipping full check")
			return
		}
	}

	// PHASE 5: Full check with protocol awareness
	checkStart := time.Now()
	var results []checker.CheckResult
	
	// Check proxies with their respective protocols
	for _, proxyWithProto := range proxies {
		result := chk.CheckProxyWithProtocol(ctx, proxyWithProto.Address, proxyWithProto.Protocol)
		results = append(results, result)
	}
	
	checkDuration := time.Since(checkStart)

	aliveCount := 0
	deadCount := 0
	aliveProxies := make([]snapshot.Proxy, 0, len(results))

	// Create protocol map for lookup
	protocolMap := make(map[string]string)
	for _, p := range proxies {
		protocolMap[p.Address] = p.Protocol
	}

	for _, result := range results {
		if result.Alive {
			aliveCount++
			protocol := protocolMap[result.Proxy]
			if protocol == "" {
				protocol = "http" // default
			}
			aliveProxies = append(aliveProxies, snapshot.Proxy{
				Address:   result.Proxy,
				Protocol:  protocol,
				Alive:     true,
				LatencyMs: result.LatencyMs,
				LastCheck: time.Now(),
			})
		} else {
			deadCount++
		}
	}

	alivePercent := 0.0
	if totalScraped > 0 {
		alivePercent = float64(aliveCount) / float64(totalScraped) * 100.0
	}

	log.Infof("Check complete: %d alive, %d dead (%.2f%% alive) in %v",
		aliveCount, deadCount, alivePercent, checkDuration)

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

