package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"llavero/internal/config"
	"llavero/internal/diagnostics"
	"llavero/internal/server"
)

func main() {
	var cfgPath string
	flags := config.Default()
	flag.StringVar(&cfgPath, "config", "", "key=value configuration file")
	flag.StringVar(&flags.Addr, "addr", flags.Addr, "TCP listen address")
	flag.StringVar(&flags.SnapshotPath, "snapshot", flags.SnapshotPath, "snapshot file; empty disables persistence")
	flag.DurationVar(&flags.SaveInterval, "save-interval", 0, "automatic snapshot interval; 0 disables it")
	flag.StringVar(&flags.AOFPath, "aof", "", "append-only file; empty disables AOF")
	flag.StringVar(&flags.AOFSync, "aof-fsync", flags.AOFSync, "AOF fsync policy: always, everysec or no")
	flag.StringVar(&flags.AuthPassword, "requirepass", "", "password required for AUTH; can also be provided via LLAVERO_REQUIREPASS")
	flag.StringVar(&flags.TLSCertPath, "tls-cert", "", "TLS PEM certificate; requires -tls-key")
	flag.StringVar(&flags.TLSKeyPath, "tls-key", "", "TLS PEM key; requires -tls-cert")
	flag.IntVar(&flags.MaxConnections, "max-connections", 0, "max simultaneous connections; 0 disables it")
	flag.DurationVar(&flags.ReadTimeout, "read-timeout", 0, "read timeout per command; 0 disables it")
	flag.DurationVar(&flags.WriteTimeout, "write-timeout", 0, "write timeout per response/pubsub; 0 disables it")
	flag.Int64Var(&flags.MaxMemoryBytes, "max-memory", 0, "approximate memory limit for keys/values; 0 disables it")
	flag.StringVar(&flags.CommandLog, "command-log", flags.CommandLog, "command logging mode: off, errors, slow or all")
	flag.DurationVar(&flags.SlowLogThreshold, "slowlog-threshold", 0, "minimum latency to log in SLOWLOG; 0 disables it")
	flag.IntVar(&flags.SlowLogMaxLen, "slowlog-max-len", flags.SlowLogMaxLen, "max entries retained in SLOWLOG")
	flag.DurationVar(&flags.ShutdownTimeout, "shutdown-timeout", flags.ShutdownTimeout, "max time to drain connections during shutdown")
	flag.Parse()

	visited := visitedFlags()
	cfg := config.Default()
	if cfgPath != "" {
		if err := cfg.ApplyFile(cfgPath); err != nil {
			log.Fatalf("failed to load config: %v", err)
		}
	}
	if err := cfg.ApplyEnv(); err != nil {
		log.Fatalf("invalid environment configuration: %v", err)
	}
	cfg = mergeVisitedFlags(cfg, flags, visited)

	// Compatibilidad con el default histórico: si se activa AOF y no se pasó
	// snapshot explícito, desactivar el snapshot por defecto.
	if cfg.AOFPath != "" && !visited["snapshot"] {
		cfg.SnapshotPath = ""
	}
	pprofServer, err := diagnostics.StartPprofFromEnv("LLAVERO_PPROF_LISTEN", "LLAVERO_PPROF_ALLOW_NON_LOOPBACK")
	if err != nil {
		log.Fatalf("failed to start pprof diagnostics: %v", err)
	}

	validatePath("snapshot", cfg.SnapshotPath)
	validatePath("aof", cfg.AOFPath)

	s, err := server.NewWithOptions(cfg.ServerOptions())
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
	}
	if err := s.Listen(); err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	log.Printf("event=start addr=%q", s.Addr())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("event=shutdown_signal signal=%q", sig)
		if pprofServer != nil {
			_ = pprofServer.Close()
		}
		if err := s.Save(); err != nil {
			log.Printf("event=snapshot_final error=%q", err)
		} else if cfg.SnapshotPath != "" {
			log.Printf("event=snapshot_final path=%q", cfg.SnapshotPath)
		}
		_ = s.Close()
	}()

	if err := s.Serve(); err != nil {
		log.Fatalf("event=server_stopped error=%q", err)
	}
	log.Println("event=shutdown_complete")
}

// validatePath rechaza rutas con componentes ".." para evitar path traversal.
func validatePath(name, path string) {
	if path == "" {
		return
	}
	cleaned := filepath.Clean(path)
	for _, part := range strings.Split(cleaned, string(filepath.Separator)) {
		if part == ".." {
			log.Fatalf("%s path must not contain '..': %s", name, path)
		}
	}
}

func visitedFlags() map[string]bool {
	out := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) { out[f.Name] = true })
	return out
}

func mergeVisitedFlags(base, flags config.Config, visited map[string]bool) config.Config {
	setString := func(name string, dst *string, val string) {
		if visited[name] {
			*dst = val
		}
	}
	setDuration := func(name string, dst *time.Duration, val time.Duration) {
		if visited[name] {
			*dst = val
		}
	}
	setInt := func(name string, dst *int, val int) {
		if visited[name] {
			*dst = val
		}
	}
	setInt64 := func(name string, dst *int64, val int64) {
		if visited[name] {
			*dst = val
		}
	}

	setString("addr", &base.Addr, flags.Addr)
	setString("snapshot", &base.SnapshotPath, flags.SnapshotPath)
	setDuration("save-interval", &base.SaveInterval, flags.SaveInterval)
	setString("aof", &base.AOFPath, flags.AOFPath)
	setString("aof-fsync", &base.AOFSync, flags.AOFSync)
	setString("requirepass", &base.AuthPassword, flags.AuthPassword)
	setString("tls-cert", &base.TLSCertPath, flags.TLSCertPath)
	setString("tls-key", &base.TLSKeyPath, flags.TLSKeyPath)
	setInt("max-connections", &base.MaxConnections, flags.MaxConnections)
	setDuration("read-timeout", &base.ReadTimeout, flags.ReadTimeout)
	setDuration("write-timeout", &base.WriteTimeout, flags.WriteTimeout)
	setInt64("max-memory", &base.MaxMemoryBytes, flags.MaxMemoryBytes)
	setString("command-log", &base.CommandLog, flags.CommandLog)
	setDuration("slowlog-threshold", &base.SlowLogThreshold, flags.SlowLogThreshold)
	setInt("slowlog-max-len", &base.SlowLogMaxLen, flags.SlowLogMaxLen)
	setDuration("shutdown-timeout", &base.ShutdownTimeout, flags.ShutdownTimeout)
	return base
}
