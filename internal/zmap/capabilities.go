package zmap

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	log "github.com/sirupsen/logrus"
)

// CheckCapabilities verifies if zmap can be executed with necessary permissions
func CheckCapabilities() error {
	// Check if running as root (easiest case)
	if os.Geteuid() == 0 {
		log.Info("Running as root - zmap capabilities satisfied")
		return nil
	}

	// Check for CAP_NET_RAW capability using getcap
	// Note: This is a basic check. In production, you might want to use
	// github.com/syndtr/gocapability/capability for more robust checking
	
	zmapPath, err := exec.LookPath("zmap")
	if err != nil {
		return fmt.Errorf("zmap binary not found in PATH")
	}

	cmd := exec.Command("getcap", zmapPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// getcap might not be available
		log.Warnf("Could not check capabilities: %v", err)
		log.Warn("Zmap may not work without root or CAP_NET_RAW capability")
		return nil // Don't fail, just warn
	}

	outputStr := string(output)
	if strings.Contains(outputStr, "cap_net_raw") && strings.Contains(outputStr, "cap_net_admin") {
		log.Info("Zmap has required capabilities (CAP_NET_RAW, CAP_NET_ADMIN)")
		return nil
	}

	return fmt.Errorf("zmap lacks required capabilities. Run: sudo setcap 'cap_net_raw,cap_net_admin=+ep' %s", zmapPath)
}

// CheckZmapBinary verifies zmap binary exists and is executable
func CheckZmapBinary(path string) error {
	// Check if file exists
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("zmap binary not found at %s", path)
		}
		return fmt.Errorf("cannot access zmap binary: %w", err)
	}

	// Check if it's a file (not directory)
	if info.IsDir() {
		return fmt.Errorf("zmap path points to a directory, not a file: %s", path)
	}

	// Check if executable
	if info.Mode()&0111 == 0 {
		return fmt.Errorf("zmap binary is not executable: %s", path)
	}

	// Try to run zmap --version
	cmd := exec.Command(path, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("zmap binary cannot be executed: %w", err)
	}

	version := strings.TrimSpace(string(output))
	log.Infof("Zmap binary found: %s", version)

	// Basic version check (zmap output format: "zmap x.y.z")
	if !strings.Contains(strings.ToLower(version), "zmap") {
		log.Warnf("Unexpected zmap version output: %s", version)
	}

	return nil
}

// VerifyZmapSetup performs complete verification of zmap setup
func VerifyZmapSetup(config ZmapConfig) error {
	// Check binary
	if err := CheckZmapBinary(config.ZmapBinary); err != nil {
		return fmt.Errorf("binary check failed: %w", err)
	}

	// Check capabilities
	if err := CheckCapabilities(); err != nil {
		return fmt.Errorf("capability check failed: %w", err)
	}

	// Check blacklist files exist
	for _, blacklist := range config.Blacklist {
		if _, err := os.Stat(blacklist); err != nil {
			log.Warnf("Blacklist file not found (will proceed without it): %s", blacklist)
		} else {
			log.Infof("Blacklist file found: %s", blacklist)
		}
	}

	// Validate ports
	if len(config.Ports) == 0 {
		return fmt.Errorf("no ports configured for scanning")
	}

	for _, port := range config.Ports {
		if port < 1 || port > 65535 {
			return fmt.Errorf("invalid port number: %d (must be 1-65535)", port)
		}
	}

	// Validate rate limit
	if config.RateLimit < 1 || config.RateLimit > 1000000 {
		return fmt.Errorf("invalid rate_limit: %d (must be 1-1000000)", config.RateLimit)
	}

	log.Info("Zmap setup verification passed")
	return nil
}

