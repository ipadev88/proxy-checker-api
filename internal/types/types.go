package types

import "time"

// Proxy represents a single proxy server
type Proxy struct {
	Address   string    `json:"address"`
	Protocol  string    `json:"protocol"` // "http" or "socks5"
	Alive     bool      `json:"alive"`
	LatencyMs int64     `json:"latency_ms"`
	LastCheck time.Time `json:"last_check"`
}

// Stats holds proxy statistics
type Stats struct {
	TotalScraped  int         `json:"total_scraped"`
	TotalAlive    int         `json:"total_alive"`
	TotalDead     int         `json:"total_dead"`
	AlivePercent  float64     `json:"alive_percent"`
	LastCheckTime time.Time   `json:"last_check_time"`
	SourceStats   interface{} `json:"source_stats,omitempty"`
	ByProtocol    map[string]struct {
		Scraped int `json:"scraped"`
		Alive   int `json:"alive"`
		Dead    int `json:"dead"`
	} `json:"by_protocol,omitempty"`
}

// Snapshot represents a point-in-time snapshot of proxy data
type Snapshot struct {
	Proxies []Proxy   `json:"proxies"`
	Stats   Stats     `json:"stats"`
	Updated time.Time `json:"updated"`
}

