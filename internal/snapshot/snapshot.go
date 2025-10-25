package snapshot

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/proxy-checker-api/internal/storage"
	"github.com/proxy-checker-api/internal/types"
	log "github.com/sirupsen/logrus"
)

// Re-export types for backward compatibility
type Proxy = types.Proxy
type Stats = types.Stats
type Snapshot = types.Snapshot

type Manager struct {
	current   atomic.Value // stores *Snapshot
	storage   storage.Storage
	persistMu sync.Mutex
	rrIndex   atomic.Uint64 // Round-robin index

	persistInterval time.Duration
	stopPersist     chan struct{}
}

func NewManager(store storage.Storage, persistIntervalSeconds int) *Manager {
	m := &Manager{
		storage:         store,
		persistInterval: time.Duration(persistIntervalSeconds) * time.Second,
		stopPersist:     make(chan struct{}),
	}

	// Initialize with empty snapshot
	m.current.Store(&types.Snapshot{
		Proxies: []types.Proxy{},
		Stats:   types.Stats{},
		Updated: time.Now(),
	})

	// Start periodic persistence
	if persistIntervalSeconds > 0 {
		go m.periodicPersist()
	}

	return m
}

// Update atomically swaps the current snapshot
func (m *Manager) Update(proxies []types.Proxy, stats types.Stats) {
	snapshot := &types.Snapshot{
		Proxies: proxies,
		Stats:   stats,
		Updated: time.Now(),
	}

	m.current.Store(snapshot)
	log.Infof("Snapshot updated: %d alive proxies", len(proxies))

	// Trigger async persistence
	go m.persist(snapshot)
}

// Get returns the current snapshot (atomic read)
func (m *Manager) Get() *types.Snapshot {
	return m.current.Load().(*types.Snapshot)
}

// GetProxy returns a single proxy using round-robin
func (m *Manager) GetProxy() (types.Proxy, bool) {
	snapshot := m.Get()
	if len(snapshot.Proxies) == 0 {
		return types.Proxy{}, false
	}

	// Round-robin selection
	idx := m.rrIndex.Add(1) % uint64(len(snapshot.Proxies))
	return snapshot.Proxies[idx], true
}

// GetProxies returns N proxies (round-robin or random)
func (m *Manager) GetProxies(n int) []types.Proxy {
	snapshot := m.Get()
	total := len(snapshot.Proxies)

	if total == 0 {
		return []types.Proxy{}
	}

	if n <= 0 || n > total {
		n = total
	}

	result := make([]types.Proxy, n)
	
	// Use round-robin for small requests
	if n <= 10 {
		startIdx := int(m.rrIndex.Add(uint64(n)) % uint64(total))
		for i := 0; i < n; i++ {
			idx := (startIdx + i) % total
			result[i] = snapshot.Proxies[idx]
		}
		return result
	}

	// Random sampling for larger requests
	indices := rand.Perm(total)
	for i := 0; i < n; i++ {
		result[i] = snapshot.Proxies[indices[i]]
	}

	return result
}

// GetAll returns all proxies
func (m *Manager) GetAll() []types.Proxy {
	snapshot := m.Get()
	// Return copy to prevent external modifications
	proxies := make([]types.Proxy, len(snapshot.Proxies))
	copy(proxies, snapshot.Proxies)
	return proxies
}

// GetStats returns current statistics
func (m *Manager) GetStats() types.Stats {
	snapshot := m.Get()
	return snapshot.Stats
}

// persist saves snapshot to storage (non-blocking)
func (m *Manager) persist(snapshot *types.Snapshot) {
	m.persistMu.Lock()
	defer m.persistMu.Unlock()

	if err := m.storage.Save(snapshot); err != nil {
		log.Errorf("Failed to persist snapshot: %v", err)
	} else {
		log.Debugf("Snapshot persisted: %d proxies", len(snapshot.Proxies))
	}
}

// periodicPersist saves snapshot at regular intervals
func (m *Manager) periodicPersist() {
	ticker := time.NewTicker(m.persistInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			snapshot := m.Get()
			m.persist(snapshot)
		case <-m.stopPersist:
			return
		}
	}
}

// LoadFromStorage loads last saved snapshot
func (m *Manager) LoadFromStorage() error {
	snapshot, err := m.storage.Load()
	if err != nil {
		return err
	}

	if snapshot != nil {
		// Filter out stale proxies (older than 1 hour)
		freshProxies := make([]Proxy, 0)
		cutoff := time.Now().Add(-1 * time.Hour)
		
		for _, p := range snapshot.Proxies {
			if p.LastCheck.After(cutoff) {
				freshProxies = append(freshProxies, p)
			}
		}

		if len(freshProxies) > 0 {
			snapshot.Proxies = freshProxies
			snapshot.Stats.TotalAlive = len(freshProxies)
			m.current.Store(snapshot)
			log.Infof("Loaded %d fresh proxies from storage", len(freshProxies))
			return nil
		}
	}

	log.Info("No fresh proxies in storage")
	return nil
}

// Close stops background tasks
func (m *Manager) Close() {
	close(m.stopPersist)
	
	// Final persist
	snapshot := m.Get()
	m.persist(snapshot)
}

