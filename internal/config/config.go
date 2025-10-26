package config

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

type Config struct {
	Aggregator AggregatorConfig `json:"aggregator"`
	Zmap       ZmapConfig       `json:"zmap"`
	Checker    CheckerConfig    `json:"checker"`
	API        APIConfig        `json:"api"`
	Storage    StorageConfig    `json:"storage"`
	Metrics    MetricsConfig    `json:"metrics"`
	Logging    LoggingConfig    `json:"logging"`

	mu       sync.RWMutex
	filePath string
}

type AggregatorConfig struct {
	IntervalSeconds int      `json:"interval_seconds"`
	Sources         []Source `json:"sources"`
	UserAgent       string   `json:"user_agent"`
}

type Source struct {
	URL      string `json:"url"`
	Type     string `json:"type"`     // "txt", "json", etc.
	Protocol string `json:"protocol"` // "http", "socks4", "socks5", "auto"
	Enabled  bool   `json:"enabled"`
}

type ZmapConfig struct {
	Enabled           bool     `json:"enabled"`
	Ports             []int    `json:"ports"`
	RateLimit         int      `json:"rate_limit"`
	Bandwidth         string   `json:"bandwidth"`
	MaxRuntimeSeconds int      `json:"max_runtime_seconds"`
	TargetRanges      []string `json:"target_ranges"`
	Blacklist         []string `json:"blacklist"`
	Interface         string   `json:"interface"`
	ZmapBinary        string   `json:"zmap_binary"`
	OutputFormat      string   `json:"output_format"`
	ZmapExtraArgs     []string `json:"zmap_extra_args"`
	CooldownSeconds   int      `json:"cooldown_seconds"`
}

type CheckerConfig struct {
	TimeoutMs                 int    `json:"timeout_ms"`
	ConcurrencyTotal          int    `json:"concurrency_total"`
	BatchSize                 int    `json:"batch_size"`
	Retries                   int    `json:"retries"`
	TestURL                   string `json:"test_url"`
	Mode                      string `json:"mode"` // "connect-only" or "full-http"
	EnableAdaptiveConcurrency bool   `json:"enable_adaptive_concurrency"`
	MaxFDUsagePercent         int    `json:"max_fd_usage_percent"`
	MaxCPUUsagePercent        int    `json:"max_cpu_usage_percent"`
	EnableFastFilter          bool   `json:"enable_fast_filter"`
	FastFilterTimeoutMs       int    `json:"fast_filter_timeout_ms"`
	FastFilterConcurrency     int    `json:"fast_filter_concurrency"`
	SocksEnabled              bool   `json:"socks_enabled"`        // Enable SOCKS checking
	SocksTimeoutMs            int    `json:"socks_timeout_ms"`     // Timeout for SOCKS checks
	SocksTestURL              string `json:"socks_test_url"`       // URL to test through SOCKS
}

type APIConfig struct {
	Addr               string `json:"addr"`
	APIKeyEnv          string `json:"api_key_env"`
	RateLimitPerMinute int    `json:"rate_limit_per_minute"`
	RateLimitPerIP     int    `json:"rate_limit_per_ip"`
	EnableAPIKeyAuth   bool   `json:"enable_api_key_auth"`
	EnableIPRateLimit  bool   `json:"enable_ip_rate_limit"`
}

type StorageConfig struct {
	Type                   string `json:"type"` // "file", "sqlite", "redis"
	Path                   string `json:"path"`
	PersistIntervalSeconds int    `json:"persist_interval_seconds"`
}

type MetricsConfig struct {
	Enabled   bool   `json:"enabled"`
	Endpoint  string `json:"endpoint"`
	Namespace string `json:"namespace"`
}

type LoggingConfig struct {
	Level  string `json:"level"`
	Format string `json:"format"`
}

var (
	globalConfig *Config
	configMu     sync.RWMutex
)

// Load reads configuration from JSON file
func Load(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config JSON: %w", err)
	}

	cfg.filePath = filePath

	// Set defaults
	if cfg.Aggregator.IntervalSeconds == 0 {
		cfg.Aggregator.IntervalSeconds = 60
	}

	// Zmap defaults (enabled by default)
	// Note: Zmap.Enabled defaults to false in JSON, but we enable it if ports are configured
	if len(cfg.Zmap.Ports) == 0 {
		cfg.Zmap.Ports = []int{8080, 80, 3128}
	}
	if cfg.Zmap.RateLimit == 0 {
		cfg.Zmap.RateLimit = 10000
	}
	if cfg.Zmap.MaxRuntimeSeconds == 0 {
		cfg.Zmap.MaxRuntimeSeconds = 3600
	}
	if cfg.Zmap.ZmapBinary == "" {
		cfg.Zmap.ZmapBinary = "/usr/local/bin/zmap"
	}
	if cfg.Zmap.OutputFormat == "" {
		cfg.Zmap.OutputFormat = "csv"
	}
	if cfg.Zmap.CooldownSeconds == 0 {
		cfg.Zmap.CooldownSeconds = 3600
	}

	// Checker defaults
	if cfg.Checker.TimeoutMs == 0 {
		cfg.Checker.TimeoutMs = 15000
	}
	if cfg.Checker.ConcurrencyTotal == 0 {
		cfg.Checker.ConcurrencyTotal = 20000
	}
	if cfg.Checker.BatchSize == 0 {
		cfg.Checker.BatchSize = 2000
	}
	if cfg.Checker.Mode == "" {
		cfg.Checker.Mode = "full-http"
	}
	if cfg.Checker.TestURL == "" {
		cfg.Checker.TestURL = "https://www.google.com/generate_204"
	}
	if cfg.Checker.FastFilterTimeoutMs == 0 {
		cfg.Checker.FastFilterTimeoutMs = 2000
	}
	if cfg.Checker.FastFilterConcurrency == 0 {
		cfg.Checker.FastFilterConcurrency = 50000
	}
	if cfg.Checker.SocksTimeoutMs == 0 {
		cfg.Checker.SocksTimeoutMs = 8000
	}
	if cfg.Checker.SocksTestURL == "" {
		cfg.Checker.SocksTestURL = "https://www.google.com/generate_204"
	}
	if cfg.API.Addr == "" {
		cfg.API.Addr = ":8083"
	}
	if cfg.API.RateLimitPerMinute == 0 {
		cfg.API.RateLimitPerMinute = 1200
	}
	if cfg.Storage.Type == "" {
		cfg.Storage.Type = "file"
	}
	if cfg.Storage.Path == "" {
		cfg.Storage.Path = "/data/proxies.json"
	}
	if cfg.Storage.PersistIntervalSeconds == 0 {
		cfg.Storage.PersistIntervalSeconds = 300
	}
	if cfg.Metrics.Namespace == "" {
		cfg.Metrics.Namespace = "proxychecker"
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}

	// Validate
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	configMu.Lock()
	globalConfig = &cfg
	configMu.Unlock()

	return &cfg, nil
}

// Reload reloads configuration from file
func (c *Config) Reload() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	newCfg, err := Load(c.filePath)
	if err != nil {
		return err
	}

	*c = *newCfg
	return nil
}

// Validate checks configuration validity
func (c *Config) Validate() error {
	// Zmap validation
	if c.Zmap.Enabled {
		if len(c.Zmap.Ports) == 0 {
			return fmt.Errorf("zmap enabled but no ports configured")
		}
		for _, port := range c.Zmap.Ports {
			if port < 1 || port > 65535 {
				return fmt.Errorf("invalid zmap port: %d (must be 1-65535)", port)
			}
		}
		if c.Zmap.RateLimit < 1 || c.Zmap.RateLimit > 1000000 {
			return fmt.Errorf("zmap rate_limit must be between 1 and 1000000")
		}
		if c.Zmap.MaxRuntimeSeconds < 1 || c.Zmap.MaxRuntimeSeconds > 86400 {
			return fmt.Errorf("zmap max_runtime_seconds must be between 1 and 86400 (24 hours)")
		}
	}

	// Checker validation
	if c.Checker.ConcurrencyTotal < 1 || c.Checker.ConcurrencyTotal > 100000 {
		return fmt.Errorf("concurrency_total must be between 1 and 100000")
	}
	if c.Checker.TimeoutMs < 100 || c.Checker.TimeoutMs > 300000 {
		return fmt.Errorf("timeout_ms must be between 100 and 300000")
	}
	if c.Checker.Mode != "connect-only" && c.Checker.Mode != "full-http" {
		return fmt.Errorf("mode must be 'connect-only' or 'full-http'")
	}
	if c.Checker.EnableFastFilter {
		if c.Checker.FastFilterTimeoutMs < 100 || c.Checker.FastFilterTimeoutMs > 30000 {
			return fmt.Errorf("fast_filter_timeout_ms must be between 100 and 30000")
		}
		if c.Checker.FastFilterConcurrency < 1 || c.Checker.FastFilterConcurrency > 200000 {
			return fmt.Errorf("fast_filter_concurrency must be between 1 and 200000")
		}
	}

	// Storage validation
	if c.Storage.Type != "file" && c.Storage.Type != "sqlite" && c.Storage.Type != "redis" {
		return fmt.Errorf("storage type must be 'file', 'sqlite', or 'redis'")
	}

	return nil
}

// GetGlobal returns global config instance
func GetGlobal() *Config {
	configMu.RLock()
	defer configMu.RUnlock()
	return globalConfig
}

