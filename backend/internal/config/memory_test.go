package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemoryConfigLoadsBindingsAndDefaults(t *testing.T) {
	resetViperWithJWTSecret(t)
	file := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(file, []byte("memory:\n  enabled: true\n  bindings:\n    - user_id: 42\n      base_url: http://127.0.0.1:15170\n      data_dir: /srv/respire/user42\n"), 0600))
	t.Setenv("CONFIG_FILE", file)
	cfg, err := Load()
	require.NoError(t, err)
	require.True(t, cfg.Memory.Enabled)
	require.Equal(t, 300, cfg.Memory.RecallTimeoutMS)
	require.Equal(t, 120000, cfg.Memory.ArchiveTimeoutMS)
	require.Len(t, cfg.Memory.Bindings, 1)
	require.Equal(t, int64(42), cfg.Memory.Bindings[0].UserID)
}

func TestMemoryConfigRejectsUnsafeOrIncompleteBinding(t *testing.T) {
	resetViperWithJWTSecret(t)
	cfg, err := Load()
	require.NoError(t, err)
	base := cfg.Memory
	base.Enabled = true
	base.Bindings = []MemoryRuntimeBinding{{UserID: 42, BaseURL: "http://127.0.0.1:15170", DataDir: "/srv/respire/u42"}}
	require.NoError(t, base.Validate())
	for _, tc := range []struct {
		name   string
		change func(*MemoryConfig)
	}{
		{"no users", func(c *MemoryConfig) { c.Bindings = nil }},
		{"body-like untrusted user", func(c *MemoryConfig) { c.Bindings[0].UserID = 0 }},
		{"external endpoint", func(c *MemoryConfig) { c.Bindings[0].BaseURL = "http://103.236.85.237:15170" }},
		{"credentials", func(c *MemoryConfig) { c.Bindings[0].BaseURL = "http://secret@127.0.0.1:15170" }},
		{"wrong path", func(c *MemoryConfig) { c.Bindings[0].BaseURL = "http://127.0.0.1:15170/rpc" }},
		{"invalid port", func(c *MemoryConfig) { c.Bindings[0].BaseURL = "http://127.0.0.1:70000" }},
		{"relative library", func(c *MemoryConfig) { c.Bindings[0].DataDir = "./user42" }},
		{"relative manifest", func(c *MemoryConfig) { c.BindingsFile = "./bindings.json" }},
		{"shared loopback alias", func(c *MemoryConfig) {
			c.Bindings = append(c.Bindings, MemoryRuntimeBinding{UserID: 43, BaseURL: "http://[::1]:15170", DataDir: "/srv/respire/u43"})
		}},
		{"shared library", func(c *MemoryConfig) {
			c.Bindings = append(c.Bindings, MemoryRuntimeBinding{UserID: 43, BaseURL: "http://127.0.0.1:15171", DataDir: "/srv/respire/u42"})
		}},
		{"lease too short", func(c *MemoryConfig) { c.LeaseSeconds = 125 }},
		{"archive timeout above adapter", func(c *MemoryConfig) { c.ArchiveTimeoutMS = 120001 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := base
			c.Bindings = append([]MemoryRuntimeBinding(nil), base.Bindings...)
			tc.change(&c)
			require.Error(t, c.Validate())
		})
	}
}

func TestMemoryConfigReadsProvisionedBindingsFile(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "bindings.json")
	require.NoError(t, os.WriteFile(filename, []byte(`[{"user_id":43,"base_url":"http://127.0.0.1:15171","data_dir":"/srv/respire/u43"}]`), 0600))
	bindings, err := ReadMemoryRuntimeBindings(filename)
	require.NoError(t, err)
	require.Equal(t, []MemoryRuntimeBinding{{UserID: 43, BaseURL: "http://127.0.0.1:15171", DataDir: "/srv/respire/u43"}}, bindings)
	require.NoError(t, os.WriteFile(filename, []byte(`[{"user_id":43`), 0600))
	_, err = ReadMemoryRuntimeBindings(filename)
	require.Error(t, err, "a partial publication must never admit a tenant")
}
