package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/proxy-checker-api/internal/aggregator"
	"github.com/proxy-checker-api/internal/checker"
	"github.com/proxy-checker-api/internal/config"
	"github.com/proxy-checker-api/internal/metrics"
	"github.com/proxy-checker-api/internal/snapshot"
	log "github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
)

type Server struct {
	config      *config.Config
	snapshot    *snapshot.Manager
	metrics     *metrics.Collector
	aggregator  *aggregator.Aggregator
	checker     *checker.Checker
	router      *gin.Engine
	httpServer  *http.Server
	rateLimiter *RateLimiter
}

type RateLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	rate     rate.Limit
	burst    int
}

func NewRateLimiter(requestsPerMinute int) *RateLimiter {
	rps := float64(requestsPerMinute) / 60.0
	return &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     rate.Limit(rps),
		burst:    requestsPerMinute / 10, // Allow bursts
	}
}

func (rl *RateLimiter) GetLimiter(key string) *rate.Limiter {
	rl.mu.RLock()
	limiter, exists := rl.limiters[key]
	rl.mu.RUnlock()

	if exists {
		return limiter
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Double-check after acquiring write lock
	if limiter, exists := rl.limiters[key]; exists {
		return limiter
	}

	limiter = rate.NewLimiter(rl.rate, rl.burst)
	rl.limiters[key] = limiter

	return limiter
}

func NewServer(cfg *config.Config, snap *snapshot.Manager, metricsCollector *metrics.Collector,
	agg *aggregator.Aggregator, chk *checker.Checker) *Server {

	if cfg.Logging.Level == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())

	s := &Server{
		config:      cfg,
		snapshot:    snap,
		metrics:     metricsCollector,
		aggregator:  agg,
		checker:     chk,
		router:      router,
		rateLimiter: NewRateLimiter(cfg.API.RateLimitPerMinute),
	}

	s.setupRoutes()

	return s
}

func (s *Server) setupRoutes() {
	// Middleware
	s.router.Use(s.loggingMiddleware())
	s.router.Use(s.metricsMiddleware())

	// Public endpoints
	s.router.GET("/health", s.handleHealth)

	// Metrics endpoint (usually scraped by Prometheus)
	if s.config.Metrics.Enabled {
		s.router.GET(s.config.Metrics.Endpoint, gin.WrapH(promhttp.Handler()))
	}

	// Protected endpoints
	protected := s.router.Group("/")
	if s.config.API.EnableAPIKeyAuth {
		protected.Use(s.authMiddleware())
	}
	if s.config.API.EnableIPRateLimit {
		protected.Use(s.rateLimitMiddleware())
	}

	protected.GET("/get-proxy", s.handleGetProxy)
	protected.GET("/stat", s.handleStat)
	protected.POST("/reload", s.handleReload)
}

func (s *Server) Start() error {
	s.httpServer = &http.Server{
		Addr:         s.config.API.Addr,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Infof("Starting API server on %s", s.config.API.Addr)
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	log.Info("Shutting down API server...")
	return s.httpServer.Shutdown(ctx)
}

// Middleware

func (s *Server) loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		duration := time.Since(start)
		statusCode := c.Writer.Status()

		log.WithFields(log.Fields{
			"method":   c.Request.Method,
			"path":     path,
			"status":   statusCode,
			"duration": duration.Milliseconds(),
			"ip":       c.ClientIP(),
		}).Info("API request")
	}
}

func (s *Server) metricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		c.Next()

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer.Status())

		s.metrics.RecordAPIRequest(method, path, status)
		s.metrics.RecordAPIDuration(method, path, duration)
	}
}

func (s *Server) authMiddleware() gin.HandlerFunc {
	expectedKey := os.Getenv(s.config.API.APIKeyEnv)
	if expectedKey == "" {
		log.Warn("API key not set in environment, authentication disabled")
	}

	return func(c *gin.Context) {
		if expectedKey == "" {
			c.Next()
			return
		}

		// Check header first
		apiKey := c.GetHeader("X-Api-Key")
		if apiKey == "" {
			// Check query parameter
			apiKey = c.Query("key")
		}

		if apiKey != expectedKey {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid or missing API key",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

func (s *Server) rateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		limiter := s.rateLimiter.GetLimiter(ip)

		if !limiter.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "Rate limit exceeded",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// Handlers

func (s *Server) handleHealth(c *gin.Context) {
	c.String(http.StatusOK, "ok")
}

func (s *Server) handleGetProxy(c *gin.Context) {
	snap := s.snapshot.Get()
	if len(snap.Proxies) == 0 {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "No alive proxies available",
		})
		return
	}

	// Parse parameters
	all := c.Query("all") == "1"
	limitStr := c.Query("limit")
	format := c.Query("format")
	acceptHeader := c.GetHeader("Accept")

	wantsJSON := format == "json" || strings.Contains(acceptHeader, "application/json")

	var proxies []snapshot.Proxy

	if all {
		proxies = s.snapshot.GetAll()
	} else if limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit < 1 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid limit parameter",
			})
			return
		}
		proxies = s.snapshot.GetProxies(limit)
	} else {
		// Default: return single proxy
		proxy, ok := s.snapshot.GetProxy()
		if !ok {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "No proxies available",
			})
			return
		}
		proxies = []snapshot.Proxy{proxy}
	}

	if wantsJSON {
		c.JSON(http.StatusOK, gin.H{
			"total":   len(snap.Proxies),
			"alive":   snap.Stats.TotalAlive,
			"proxies": proxies,
		})
	} else {
		// Plain text format (one per line)
		var result strings.Builder
		for _, p := range proxies {
			result.WriteString(p.Address)
			result.WriteString("\n")
		}
		c.String(http.StatusOK, result.String())
	}
}

func (s *Server) handleStat(c *gin.Context) {
	stats := s.snapshot.GetStats()
	snap := s.snapshot.Get()

	response := gin.H{
		"total_scraped": stats.TotalScraped,
		"total_alive":   stats.TotalAlive,
		"total_dead":    stats.TotalDead,
		"alive_percent": fmt.Sprintf("%.2f%%", stats.AlivePercent),
		"last_check":    stats.LastCheckTime.Format(time.RFC3339),
		"updated":       snap.Updated.Format(time.RFC3339),
	}

	if stats.SourceStats != nil {
		response["sources"] = stats.SourceStats
	}

	c.JSON(http.StatusOK, response)
}

func (s *Server) handleReload(c *gin.Context) {
	log.Info("Manual reload triggered via API")

	go func() {
		ctx := context.Background()

		// Re-aggregate
		proxies, sourceStats, err := s.aggregator.Aggregate(ctx)
		if err != nil {
			log.Errorf("Reload aggregation failed: %v", err)
			return
		}

		// Re-check
		results := s.checker.CheckProxies(ctx, proxies)

		aliveCount := 0
		aliveProxies := make([]snapshot.Proxy, 0)

		for _, result := range results {
			if result.Alive {
				aliveCount++
				aliveProxies = append(aliveProxies, snapshot.Proxy{
					Address:   result.Proxy,
					Alive:     true,
					LatencyMs: result.LatencyMs,
					LastCheck: time.Now(),
				})
			}
		}

		stats := snapshot.Stats{
			TotalScraped:  len(proxies),
			TotalAlive:    aliveCount,
			TotalDead:     len(proxies) - aliveCount,
			AlivePercent:  float64(aliveCount) / float64(len(proxies)) * 100.0,
			LastCheckTime: time.Now(),
			SourceStats:   sourceStats,
		}

		s.snapshot.Update(aliveProxies, stats)
		log.Info("Reload complete")
	}()

	c.JSON(http.StatusOK, gin.H{
		"message": "Reload triggered",
	})
}

