package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "llavero.conf")
	if err := os.WriteFile(path, []byte(`
# comentario
addr=127.0.0.1:1234
save-interval=30s
max_connections=20
max_memory=1024
slowlog-threshold=5ms
shutdown_timeout=2s
`), 0o644); err != nil {
		t.Fatalf("WriteFile -> %v", err)
	}
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile -> %v", err)
	}
	if cfg.Addr != "127.0.0.1:1234" || cfg.SaveInterval != 30*time.Second || cfg.MaxConnections != 20 {
		t.Fatalf("cfg = %#v", cfg)
	}
	if cfg.MaxMemoryBytes != 1024 || cfg.SlowLogThreshold != 5*time.Millisecond || cfg.ShutdownTimeout != 2*time.Second {
		t.Fatalf("cfg = %#v", cfg)
	}
}

func TestApplyEnv(t *testing.T) {
	t.Setenv("LLAVERO_ADDR", "127.0.0.1:7777")
	t.Setenv("LLAVERO_REQUIREPASS", "secret")
	t.Setenv("LLAVERO_READ_TIMEOUT", "10s")
	cfg := Default()
	if err := cfg.ApplyEnv(); err != nil {
		t.Fatalf("ApplyEnv -> %v", err)
	}
	if cfg.Addr != "127.0.0.1:7777" || cfg.AuthPassword != "secret" || cfg.ReadTimeout != 10*time.Second {
		t.Fatalf("cfg = %#v", cfg)
	}
}

func TestSetRejectsUnknownKey(t *testing.T) {
	var cfg Config
	if err := cfg.Set("nope", "x"); err == nil {
		t.Fatal("Set aceptó clave desconocida")
	}
}

func TestMergeOverlay(t *testing.T) {
	cfg := Default()
	merged := cfg.MergeOverlay(Config{Addr: "x", MaxConnections: 4})
	if merged.Addr != "x" || merged.MaxConnections != 4 {
		t.Fatalf("merged = %#v", merged)
	}
	if merged.SnapshotPath != cfg.SnapshotPath {
		t.Fatalf("perdió defaults: %#v", merged)
	}
}

func TestApplyFileCanDisableSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "llavero.conf")
	if err := os.WriteFile(path, []byte("snapshot=\n"), 0o644); err != nil {
		t.Fatalf("WriteFile -> %v", err)
	}
	cfg := Default()
	if err := cfg.ApplyFile(path); err != nil {
		t.Fatalf("ApplyFile -> %v", err)
	}
	if cfg.SnapshotPath != "" {
		t.Fatalf("SnapshotPath = %q, quería vacío", cfg.SnapshotPath)
	}
}
