package aggregator

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/proxy-checker-api/internal/config"
	"github.com/proxy-checker-api/internal/metrics"
	log "github.com/sirupsen/logrus"
)

var (
	// Regex to match proxy formats: IP:PORT or http://IP:PORT or socks4://IP:PORT or socks5://IP:PORT
	proxyRegex = regexp.MustCompile(`(?:(socks5|socks4|https?)://)?(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}):(\d{2,5})`)
)

type Aggregator struct {
	config  config.AggregatorConfig
	metrics *metrics.Collector
	client  *http.Client
}

type SourceStats struct {
	URL          string
	ProxiesFound int
	Error        string
}

type ProxyWithProtocol struct {
	Address  string
	Protocol string // "http", "socks4", "socks5"
}

func NewAggregator(cfg config.AggregatorConfig, metricsCollector *metrics.Collector) *Aggregator {
	return &Aggregator{
		config:  cfg,
		metrics: metricsCollector,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 2,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// Aggregate fetches proxies from all enabled sources
func (a *Aggregator) Aggregate(ctx context.Context) ([]ProxyWithProtocol, map[string]SourceStats, error) {
	enabledSources := make([]config.Source, 0)
	for _, source := range a.config.Sources {
		if source.Enabled {
			enabledSources = append(enabledSources, source)
		}
	}

	if len(enabledSources) == 0 {
		return nil, nil, fmt.Errorf("no enabled sources")
	}

	log.Infof("Fetching from %d sources", len(enabledSources))

	var wg sync.WaitGroup
	resultChan := make(chan []ProxyWithProtocol, len(enabledSources))
	statsChan := make(chan SourceStats, len(enabledSources))

	// Fetch from all sources concurrently
	for _, source := range enabledSources {
		wg.Add(1)
		go func(src config.Source) {
			defer wg.Done()

			startTime := time.Now()
			proxies, err := a.fetchSource(ctx, src)
			duration := time.Since(startTime)

			stat := SourceStats{
				URL:          src.URL,
				ProxiesFound: len(proxies),
			}

			if err != nil {
				stat.Error = err.Error()
				log.Warnf("Source %s failed: %v (took %v)", src.URL, err, duration)
			} else {
				log.Infof("Source %s returned %d proxies (took %v)", src.URL, len(proxies), duration)
			}

			a.metrics.RecordProxiesScraped(src.URL, len(proxies))

			resultChan <- proxies
			statsChan <- stat
		}(source)
	}

	// Wait for all fetches to complete
	wg.Wait()
	close(resultChan)
	close(statsChan)

	// Collect results
	allProxies := make([]ProxyWithProtocol, 0)
	for proxies := range resultChan {
		allProxies = append(allProxies, proxies...)
	}

	sourceStats := make(map[string]SourceStats)
	for stat := range statsChan {
		sourceStats[stat.URL] = stat
	}

	// Deduplicate
	unique := deduplicateProxies(allProxies)
	log.Infof("Deduplicated: %d -> %d unique proxies", len(allProxies), len(unique))

	return unique, sourceStats, nil
}

func (a *Aggregator) fetchSource(ctx context.Context, source config.Source) ([]ProxyWithProtocol, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", source.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if a.config.UserAgent != "" {
		req.Header.Set("User-Agent", a.config.UserAgent)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// Limit body read to 10MB
	limitedReader := io.LimitReader(resp.Body, 10*1024*1024)

	// Detect protocol from source config or URL
	defaultProtocol := source.Protocol
	if defaultProtocol == "" || defaultProtocol == "auto" {
		// Try to detect from URL
		if strings.Contains(strings.ToLower(source.URL), "socks5") {
			defaultProtocol = "socks5"
		} else if strings.Contains(strings.ToLower(source.URL), "socks4") {
			defaultProtocol = "socks4"
		} else {
			defaultProtocol = "http"
		}
	}

	return parseProxies(limitedReader, defaultProtocol)
}

func parseProxies(r io.Reader, defaultProtocol string) ([]ProxyWithProtocol, error) {
	proxies := make([]ProxyWithProtocol, 0)
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Extract proxy using regex
		matches := proxyRegex.FindStringSubmatch(line)
		if len(matches) >= 4 {
			protocol := matches[1] // Could be empty, "http", "https", "socks4", "socks5"
			ip := matches[2]
			port := matches[3]
			
			// Determine protocol
			detectedProtocol := defaultProtocol
			if protocol != "" {
				// Protocol explicitly specified in the line
				if protocol == "socks5" {
					detectedProtocol = "socks5"
				} else if protocol == "socks4" {
					detectedProtocol = "socks4"
				} else {
					detectedProtocol = "http"
				}
			}
			
			proxy := ProxyWithProtocol{
				Address:  fmt.Sprintf("%s:%s", ip, port),
				Protocol: detectedProtocol,
			}
			proxies = append(proxies, proxy)
		}
	}

	if err := scanner.Err(); err != nil {
		return proxies, fmt.Errorf("scan: %w", err)
	}

	return proxies, nil
}

func deduplicateProxies(proxies []ProxyWithProtocol) []ProxyWithProtocol {
	seen := make(map[string]struct{}, len(proxies))
	unique := make([]ProxyWithProtocol, 0, len(proxies))

	for _, proxy := range proxies {
		// Key by address + protocol
		key := fmt.Sprintf("%s|%s", strings.ToLower(strings.TrimSpace(proxy.Address)), proxy.Protocol)
		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			unique = append(unique, proxy)
		}
	}

	return unique
}

