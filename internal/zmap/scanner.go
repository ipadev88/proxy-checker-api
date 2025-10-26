package zmap

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/proxy-checker-api/internal/aggregator"
	"github.com/proxy-checker-api/internal/metrics"
	log "github.com/sirupsen/logrus"
)

type ZmapScanner struct {
	config       ZmapConfig
	metrics      *metrics.Collector
	mu           sync.RWMutex
	lastScanTime time.Time
	lastDuration time.Duration
	lastCandidates int
	totalScans   int64
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

func NewZmapScanner(cfg ZmapConfig, metricsCollector *metrics.Collector) *ZmapScanner {
	return &ZmapScanner{
		config:  cfg,
		metrics: metricsCollector,
	}
}

// Scan runs zmap for all configured ports and returns candidate proxy addresses
func (z *ZmapScanner) Scan(ctx context.Context) ([]string, error) {
	proxiesWithProto, err := z.ScanWithProtocol(ctx)
	if err != nil {
		return nil, err
	}
	
	// Convert to string addresses
	addresses := make([]string, len(proxiesWithProto))
	for i, p := range proxiesWithProto {
		addresses[i] = p.Address
	}
	
	return addresses, nil
}

// ScanWithProtocol runs zmap for all configured ports and returns candidate proxies with protocol detection
func (z *ZmapScanner) ScanWithProtocol(ctx context.Context) ([]aggregator.ProxyWithProtocol, error) {
	if !z.config.Enabled {
		return nil, fmt.Errorf("zmap scanning is disabled")
	}

	log.Infof("Starting zmap scan on ports %v", z.config.Ports)
	startTime := time.Now()

	allCandidates := make([]aggregator.ProxyWithProtocol, 0)
	var mu sync.Mutex

	// Scan each port sequentially to avoid overwhelming the network
	for _, port := range z.config.Ports {
		candidates, protocol, err := z.scanPortWithProtocol(ctx, port)
		if err != nil {
			log.Errorf("Failed to scan port %d: %v", port, err)
			if z.metrics != nil {
				z.metrics.RecordZmapScan(port, "error")
			}
			continue
		}

		mu.Lock()
		for _, addr := range candidates {
			allCandidates = append(allCandidates, aggregator.ProxyWithProtocol{
				Address:  addr,
				Protocol: protocol,
			})
		}
		mu.Unlock()

		log.Infof("Port %d scan complete: %d candidates found (protocol: %s)", port, len(candidates), protocol)
		if z.metrics != nil {
			z.metrics.RecordZmapScan(port, "success")
			z.metrics.RecordZmapCandidates(port, len(candidates))
		}
	}

	// Deduplicate (based on address+protocol)
	uniqueCandidates := deduplicateProxiesWithProtocol(allCandidates)
	duration := time.Since(startTime)

	// Update stats
	z.mu.Lock()
	z.lastScanTime = startTime
	z.lastDuration = duration
	z.lastCandidates = len(uniqueCandidates)
	z.totalScans++
	z.mu.Unlock()

	log.Infof("Zmap scan complete: %d unique candidates in %v", len(uniqueCandidates), duration)

	if z.metrics != nil {
		z.metrics.RecordZmapDuration(duration.Seconds())
	}

	return uniqueCandidates, nil
}

// scanPort runs zmap for a single port (legacy method)
func (z *ZmapScanner) scanPort(ctx context.Context, port int) ([]string, error) {
	candidates, _, err := z.scanPortWithProtocol(ctx, port)
	return candidates, err
}

// scanPortWithProtocol runs zmap for a single port and determines protocol
func (z *ZmapScanner) scanPortWithProtocol(ctx context.Context, port int) ([]string, string, error) {
	// Create temporary output file
	outputFile := filepath.Join(os.TempDir(), fmt.Sprintf("zmap_port_%d_%d.csv", port, time.Now().Unix()))
	defer os.Remove(outputFile)

	// Build zmap command
	cmd := z.buildZmapCmd(port, outputFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(z.config.MaxRuntimeSeconds)*time.Second)
	defer cancel()

	cmd = exec.CommandContext(cmdCtx, cmd.Path, cmd.Args[1:]...)

	log.Infof("Executing: %s", strings.Join(cmd.Args, " "))
	
	startTime := time.Now()
	if err := cmd.Run(); err != nil {
		if cmdCtx.Err() == context.DeadlineExceeded {
			log.Warnf("Zmap scan on port %d timed out after %ds", port, z.config.MaxRuntimeSeconds)
		} else {
			return nil, fmt.Errorf("zmap command failed: %w", err)
		}
	}

	duration := time.Since(startTime)
	log.Infof("Port %d zmap scan completed in %v", port, duration)

	// Parse output and detect protocol
	candidates, protocol, err := z.parseZmapOutputWithProtocol(outputFile, port)
	if err != nil {
		return nil, "", fmt.Errorf("parse zmap output: %w", err)
	}

	return candidates, protocol, nil
}

// buildZmapCmd constructs the zmap command with all flags
func (z *ZmapScanner) buildZmapCmd(port int, outputFile string) *exec.Cmd {
	args := []string{
		z.config.ZmapBinary,
		"-p", fmt.Sprintf("%d", port),
		"-r", fmt.Sprintf("%d", z.config.RateLimit),
		"-o", outputFile,
		"--output-fields=saddr",
		"--output-module=csv",
	}

	// Add bandwidth limit if specified
	if z.config.Bandwidth != "" {
		args = append(args, "-B", z.config.Bandwidth)
	}

	// Add timeout
	if z.config.MaxRuntimeSeconds > 0 {
		args = append(args, "-T", fmt.Sprintf("%d", z.config.MaxRuntimeSeconds))
	}

	// Add blacklist files
	for _, blacklist := range z.config.Blacklist {
		if _, err := os.Stat(blacklist); err == nil {
			args = append(args, "-b", blacklist)
		} else {
			log.Warnf("Blacklist file not found: %s", blacklist)
		}
	}

	// Add interface if specified
	if z.config.Interface != "" {
		args = append(args, "-i", z.config.Interface)
	}

	// Add extra args
	args = append(args, z.config.ZmapExtraArgs...)

	// Add target ranges (if empty, scans all)
	if len(z.config.TargetRanges) > 0 {
		args = append(args, z.config.TargetRanges...)
	}

	return exec.Command(args[0], args[1:]...)
}

// parseZmapOutput reads the CSV output and extracts IP:PORT strings (legacy)
func (z *ZmapScanner) parseZmapOutput(outputFile string, port int) ([]string, error) {
	candidates, _, err := z.parseZmapOutputWithProtocol(outputFile, port)
	return candidates, err
}

// parseZmapOutputWithProtocol reads the CSV output and extracts IP:PORT strings with protocol detection
func (z *ZmapScanner) parseZmapOutputWithProtocol(outputFile string, port int) ([]string, string, error) {
	file, err := os.Open(outputFile)
	if err != nil {
		return nil, fmt.Errorf("open output file: %w", err)
	}
	defer file.Close()

	proxies := make([]string, 0)
	scanner := bufio.NewScanner(file)

	// Skip header line if present
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and header
		if line == "" || line == "saddr" || strings.HasPrefix(line, "#") {
			continue
		}

		// CSV format: just IP address
		ip := line

		// Validate IP format (basic check)
		if !isValidIP(ip) {
			log.Debugf("Skipping invalid IP: %s", ip)
			continue
		}

		// Construct proxy address
		proxy := fmt.Sprintf("%s:%d", ip, port)
		proxies = append(proxies, proxy)
	}

	if err := scanner.Err(); err != nil {
		return proxies, "", fmt.Errorf("scan file: %w", err)
	}

	// Determine protocol based on port
	protocol := mapPortToProtocol(port)

	log.Infof("Parsed %d proxies from %s (%d lines), protocol: %s", len(proxies), outputFile, lineNum, protocol)

	return proxies, protocol, nil
}

// mapPortToProtocol determines the proxy protocol based on port number
func mapPortToProtocol(port int) string {
	switch port {
	case 80, 8080, 3128, 8888, 9090:
		return "http"
	case 1080:
		return "socks5"
	case 1081:
		return "socks4"
	default:
		// Default to http for unknown ports
		return "http"
	}
}

// isValidIP performs basic IP address validation
func isValidIP(ip string) bool {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return false
	}

	for _, part := range parts {
		if part == "" || len(part) > 3 {
			return false
		}
		// Basic check - more thorough validation could be added
		if !strings.ContainsAny(part, "0123456789") {
			return false
		}
	}

	return true
}

// deduplicateProxies removes duplicate proxy addresses
func deduplicateProxies(proxies []string) []string {
	seen := make(map[string]struct{}, len(proxies))
	unique := make([]string, 0, len(proxies))

	for _, proxy := range proxies {
		normalized := strings.ToLower(strings.TrimSpace(proxy))
		if _, exists := seen[normalized]; !exists {
			seen[normalized] = struct{}{}
			unique = append(unique, proxy)
		}
	}

	return unique
}

// deduplicateProxiesWithProtocol removes duplicate proxy addresses with protocol awareness
func deduplicateProxiesWithProtocol(proxies []aggregator.ProxyWithProtocol) []aggregator.ProxyWithProtocol {
	seen := make(map[string]struct{}, len(proxies))
	unique := make([]aggregator.ProxyWithProtocol, 0, len(proxies))

	for _, proxy := range proxies {
		// Key by address + protocol
		key := strings.ToLower(strings.TrimSpace(proxy.Address)) + "|" + proxy.Protocol
		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			unique = append(unique, proxy)
		}
	}

	return unique
}

// GetStats returns current scanner statistics
func (z *ZmapScanner) GetStats() map[string]interface{} {
	z.mu.RLock()
	defer z.mu.RUnlock()

	return map[string]interface{}{
		"enabled":            z.config.Enabled,
		"ports":              z.config.Ports,
		"last_scan_time":     z.lastScanTime,
		"last_scan_duration": z.lastDuration.Seconds(),
		"candidates_found":   z.lastCandidates,
		"total_scans":        z.totalScans,
	}
}

// LastScanTime returns the timestamp of the last scan
func (z *ZmapScanner) LastScanTime() time.Time {
	z.mu.RLock()
	defer z.mu.RUnlock()
	return z.lastScanTime
}

// LastScanDuration returns the duration of the last scan
func (z *ZmapScanner) LastScanDuration() time.Duration {
	z.mu.RLock()
	defer z.mu.RUnlock()
	return z.lastDuration
}

// LastCandidatesCount returns the number of candidates from the last scan
func (z *ZmapScanner) LastCandidatesCount() int {
	z.mu.RLock()
	defer z.mu.RUnlock()
	return z.lastCandidates
}

// TotalScans returns the total number of scans performed
func (z *ZmapScanner) TotalScans() int64 {
	z.mu.RLock()
	defer z.mu.RUnlock()
	return z.totalScans
}

