package config

import (
	"sync"
)

// Manager holds the live configuration, allowing the admin API to
// update it at runtime while handlers read it per request.
type Manager struct {
	mu   sync.RWMutex
	cfg  Config
	path string
}

// NewManager wraps a loaded config bound to its file path.
func NewManager(cfg Config, path string) *Manager {
	return &Manager{cfg: cfg, path: path}
}

// Get returns a copy of the current config.
func (m *Manager) Get() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

// Update validates, persists, and applies a new config.
func (m *Manager) Update(cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := Save(cfg, m.path); err != nil {
		return err
	}
	m.cfg = cfg
	return nil
}
