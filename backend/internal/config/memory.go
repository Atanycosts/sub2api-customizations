package config

import (
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
)

// MemoryConfig is server deployment configuration. Bindings are an explicit
// mapping of product users to dedicated libraries, independent of local clients.
type MemoryConfig struct {
	Enabled          bool                   `mapstructure:"enabled"`
	Bindings         []MemoryRuntimeBinding `mapstructure:"bindings"`
	RecallTimeoutMS  int                    `mapstructure:"recall_timeout_ms"`
	ArchiveTimeoutMS int                    `mapstructure:"archive_timeout_ms"`
	EnqueueTimeoutMS int                    `mapstructure:"enqueue_timeout_ms"`
	PollIntervalMS   int                    `mapstructure:"poll_interval_ms"`
	LeaseSeconds     int                    `mapstructure:"lease_seconds"`
	MaxAttempts      int                    `mapstructure:"max_attempts"`
	MaxItems         int                    `mapstructure:"max_items"`
	MaxBytes         int                    `mapstructure:"max_bytes"`
	MaxTokens        int                    `mapstructure:"max_tokens"`
	CaptureMaxBytes  int                    `mapstructure:"capture_max_bytes"`
	MaxResponseBytes int64                  `mapstructure:"max_response_bytes"`
}

type MemoryRuntimeBinding struct {
	UserID  int64  `mapstructure:"user_id"`
	BaseURL string `mapstructure:"base_url"`
	DataDir string `mapstructure:"data_dir"`
}

func (c MemoryConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if len(c.Bindings) == 0 {
		return fmt.Errorf("memory.bindings requires at least one dedicated product-user Runtime")
	}
	users, endpoints, dirs := map[int64]bool{}, map[string]bool{}, map[string]bool{}
	for i, b := range c.Bindings {
		if b.UserID <= 0 || users[b.UserID] {
			return fmt.Errorf("memory.bindings[%d].user_id must be positive and unique", i)
		}
		u, err := url.Parse(b.BaseURL)
		if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			return fmt.Errorf("memory.bindings[%d].base_url must be a loopback HTTP origin", i)
		}
		ip := net.ParseIP(u.Hostname())
		if ip == nil || !ip.IsLoopback() || u.Port() == "" {
			return fmt.Errorf("memory.bindings[%d].base_url requires a loopback IP and explicit port", i)
		}
		port, portErr := strconv.Atoi(u.Port())
		if portErr != nil || port < 1 || port > 65535 {
			return fmt.Errorf("memory.bindings[%d].base_url requires a valid port", i)
		}
		// A single loopback port may have several aliases. Reserve it for one
		// tenant, consistent with the provider's immutable-runtime binding.
		endpoint := strconv.Itoa(port)
		if endpoints[endpoint] || !filepath.IsAbs(b.DataDir) || filepath.Clean(b.DataDir) != b.DataDir || dirs[b.DataDir] {
			return fmt.Errorf("memory.bindings[%d] requires unique endpoint and unique absolute data_dir", i)
		}
		users[b.UserID], endpoints[endpoint], dirs[b.DataDir] = true, true, true
	}
	for _, p := range []struct {
		name        string
		n, min, max int64
	}{
		{"recall_timeout_ms", int64(c.RecallTimeoutMS), 1, 10000},
		{"archive_timeout_ms", int64(c.ArchiveTimeoutMS), 100, 120000},
		{"enqueue_timeout_ms", int64(c.EnqueueTimeoutMS), 1, 5000},
		{"poll_interval_ms", int64(c.PollIntervalMS), 50, 60000},
		{"lease_seconds", int64(c.LeaseSeconds), 1, 3600},
		{"max_attempts", int64(c.MaxAttempts), 1, 20},
		{"max_items", int64(c.MaxItems), 1, 32},
		{"max_bytes", int64(c.MaxBytes), 256, 65536},
		{"max_tokens", int64(c.MaxTokens), 256, 4096},
		{"capture_max_bytes", int64(c.CaptureMaxBytes), 1024, 262144},
		{"max_response_bytes", c.MaxResponseBytes, 4096, 4194304},
	} {
		if p.n < p.min || p.n > p.max {
			return fmt.Errorf("memory.%s must be between %d and %d", p.name, p.min, p.max)
		}
	}
	if int64(c.LeaseSeconds)*1000 <= int64(c.ArchiveTimeoutMS)+5000 {
		return fmt.Errorf("memory.lease_seconds must exceed archive_timeout_ms by more than 5 seconds")
	}
	return nil
}
