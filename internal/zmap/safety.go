package zmap

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
)

// DefaultBlacklistRanges returns commonly excluded IP ranges
func DefaultBlacklistRanges() []string {
	return []string{
		"0.0.0.0/8",          // "This" Network (RFC 1122)
		"10.0.0.0/8",         // Private-Use Networks (RFC 1918)
		"100.64.0.0/10",      // Shared Address Space (RFC 6598)
		"127.0.0.0/8",        // Loopback (RFC 1122)
		"169.254.0.0/16",     // Link Local (RFC 3927)
		"172.16.0.0/12",      // Private-Use Networks (RFC 1918)
		"192.0.0.0/24",       // IETF Protocol Assignments (RFC 6890)
		"192.0.2.0/24",       // Documentation (TEST-NET-1) (RFC 5737)
		"192.168.0.0/16",     // Private-Use Networks (RFC 1918)
		"198.18.0.0/15",      // Benchmarking (RFC 2544)
		"198.51.100.0/24",    // Documentation (TEST-NET-2) (RFC 5737)
		"203.0.113.0/24",     // Documentation (TEST-NET-3) (RFC 5737)
		"224.0.0.0/4",        // Multicast (RFC 3171)
		"240.0.0.0/4",        // Reserved for Future Use (RFC 1112)
		"255.255.255.255/32", // Limited Broadcast (RFC 0919)
	}
}

// LoadBlacklist loads CIDR ranges from a file
func LoadBlacklist(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open blacklist file: %w", err)
	}
	defer file.Close()

	ranges := make([]string, 0)
	scanner := bufio.NewScanner(file)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Validate CIDR notation
		if _, _, err := net.ParseCIDR(line); err != nil {
			log.Warnf("Invalid CIDR at line %d: %s", lineNum, line)
			continue
		}

		ranges = append(ranges, line)
	}

	if err := scanner.Err(); err != nil {
		return ranges, fmt.Errorf("scan blacklist file: %w", err)
	}

	log.Infof("Loaded %d CIDR ranges from blacklist: %s", len(ranges), path)

	return ranges, nil
}

// ValidateTargets checks if target ranges are safe to scan
func ValidateTargets(ranges []string) error {
	if len(ranges) == 0 {
		log.Warn("No target ranges specified - will scan entire IPv4 space. This is not recommended!")
		log.Warn("Consider specifying target_ranges in config to limit scan scope.")
		return nil
	}

	// Check for extremely broad ranges
	for _, cidr := range ranges {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			return fmt.Errorf("invalid CIDR: %s", cidr)
		}

		// Calculate network size
		ones, bits := ipNet.Mask.Size()
		networkSize := 1 << uint(bits-ones)

		// Warn if scanning more than 16M addresses (equivalent to /8)
		if networkSize > 16777216 {
			log.Warnf("Large network scan detected: %s (%d addresses)", cidr, networkSize)
			log.Warn("Ensure you have authorization to scan this range")
		}
	}

	// Check for potentially sensitive networks (basic check)
	sensitivePatterns := []string{
		".mil",  // Military
		".gov",  // Government
		".edu",  // Educational (some protection)
	}

	for _, pattern := range sensitivePatterns {
		for _, target := range ranges {
			if strings.Contains(target, pattern) {
				log.Warnf("Potentially sensitive target detected: %s", target)
				log.Warn("Ensure you have explicit authorization")
			}
		}
	}

	return nil
}

// CreateBlacklistFile creates a blacklist file with default ranges
func CreateBlacklistFile(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create blacklist file: %w", err)
	}
	defer file.Close()

	// Write header
	file.WriteString("# Zmap Blacklist - Automatically Generated\n")
	file.WriteString("# CIDR ranges to exclude from scanning\n")
	file.WriteString("# Format: one CIDR per line, # for comments\n\n")

	// Write default ranges
	for _, cidr := range DefaultBlacklistRanges() {
		file.WriteString(cidr + "\n")
	}

	log.Infof("Created default blacklist file: %s", path)

	return nil
}

// ValidateConfig performs safety checks on zmap configuration
func ValidateConfig(config ZmapConfig) error {
	// Check rate limit is reasonable
	if config.RateLimit > 50000 {
		log.Warnf("High rate limit detected: %d pps", config.RateLimit)
		log.Warn("High scan rates may trigger network security alerts")
		log.Warn("Consider using a lower rate (10000-25000 pps)")
	}

	// Warn if no blacklist specified
	if len(config.Blacklist) == 0 {
		log.Warn("No blacklist files specified in config")
		log.Warn("Private and reserved IP ranges will not be automatically excluded")
		log.Warn("Add blacklist files to config.zmap.blacklist")
	}

	// Check cooldown is reasonable
	if config.CooldownSeconds < 300 {
		log.Warnf("Short cooldown period: %d seconds", config.CooldownSeconds)
		log.Warn("Frequent scans may be flagged as malicious. Recommended: 3600s (1 hour)")
	}

	// Validate target ranges
	if err := ValidateTargets(config.TargetRanges); err != nil {
		return fmt.Errorf("invalid target ranges: %w", err)
	}

	return nil
}

