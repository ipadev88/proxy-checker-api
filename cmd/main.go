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

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start aggregation loop
	go runAggregationLoop(ctx, agg, chk, snapshotMgr, cfg.Aggregator.IntervalSeconds)

	// Start API server
	apiServer := api.NewServer(cfg, snapshotMgr, metricsCollector, agg, chk)
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

func runAggregationLoop(ctx context.Context, agg *aggregator.Aggregator, chk *checker.Checker, snap *snapshot.Manager, intervalSeconds int) {
	// Run immediately on startup
	runAggregationCycle(ctx, agg, chk, snap)

	ticker := time.NewTicker(time.Duration(intervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("Aggregation loop stopped")
			return
		case <-ticker.C:
			runAggregationCycle(ctx, agg, chk, snap)
		}
	}
}

func runAggregationCycle(ctx context.Context, agg *aggregator.Aggregator, chk *checker.Checker, snap *snapshot.Manager) {
	start := time.Now()
	log.Info("Starting aggregation cycle")

	// Fetch proxies from sources
	proxies, sourceStats, err := agg.Aggregate(ctx)
	if err != nil {
		log.Errorf("Aggregation failed: %v", err)
		return
	}

	totalScraped := len(proxies)
	log.Infof("Aggregated %d unique proxies from %d sources", totalScraped, len(sourceStats))

	if totalScraped == 0 {
		log.Warn("No proxies aggregated, skipping check cycle")
		return
	}

	// Check proxies
	checkStart := time.Now()
	results := chk.CheckProxies(ctx, proxies)
	checkDuration := time.Since(checkStart)

	aliveCount := 0
	deadCount := 0
	aliveProxies := make([]snapshot.Proxy, 0, len(results))

	for _, result := range results {
		if result.Alive {
			aliveCount++
			aliveProxies = append(aliveProxies, snapshot.Proxy{
				Address:   result.Proxy,
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

