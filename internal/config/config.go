// Package config carga configuración operativa de Llavero desde archivo y env.
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"llavero/internal/server"
)

// Config contiene la configuración del binario servidor.
type Config struct {
	Addr             string
	SnapshotPath     string
	SaveInterval     time.Duration
	AOFPath          string
	AOFSync          string
	AuthPassword     string
	TLSCertPath      string
	TLSKeyPath       string
	MaxConnections   int
	ReadTimeout      time.Duration
	WriteTimeout     time.Duration
	MaxMemoryBytes   int64
	SlowLogThreshold time.Duration
	SlowLogMaxLen    int
	ShutdownTimeout  time.Duration
}

// Default devuelve la configuración por defecto usada por cmd/llavero.
func Default() Config {
	return Config{
		Addr:            "127.0.0.1:6380",
		SnapshotPath:    "llavero.snapshot",
		AOFSync:         "always",
		SlowLogMaxLen:   128,
		ShutdownTimeout: 5 * time.Second,
	}
}

// LoadFile carga pares key=value. Líneas vacías y comentarios (#) se ignoran.
func LoadFile(path string) (Config, error) {
	cfg := Config{}
	if err := cfg.ApplyFile(path); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// ApplyFile aplica pares key=value sobre una configuración existente.
func (c *Config) ApplyFile(path string) error {
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("%s:%d: esperaba key=value", path, lineNo)
		}
		if err := c.Set(strings.TrimSpace(key), strings.TrimSpace(value)); err != nil {
			return fmt.Errorf("%s:%d: %w", path, lineNo, err)
		}
	}
	return scanner.Err()
}

// ApplyEnv aplica variables LLAVERO_* si están definidas.
func (c *Config) ApplyEnv() error {
	env := map[string]string{
		"LLAVERO_ADDR":              "addr",
		"LLAVERO_SNAPSHOT":          "snapshot",
		"LLAVERO_SAVE_INTERVAL":     "save_interval",
		"LLAVERO_AOF":               "aof",
		"LLAVERO_AOF_FSYNC":         "aof_fsync",
		"LLAVERO_REQUIREPASS":       "requirepass",
		"LLAVERO_TLS_CERT":          "tls_cert",
		"LLAVERO_TLS_KEY":           "tls_key",
		"LLAVERO_MAX_CONNECTIONS":   "max_connections",
		"LLAVERO_READ_TIMEOUT":      "read_timeout",
		"LLAVERO_WRITE_TIMEOUT":     "write_timeout",
		"LLAVERO_MAX_MEMORY":        "max_memory",
		"LLAVERO_SLOWLOG_THRESHOLD": "slowlog_threshold",
		"LLAVERO_SLOWLOG_MAX_LEN":   "slowlog_max_len",
		"LLAVERO_SHUTDOWN_TIMEOUT":  "shutdown_timeout",
	}
	for name, key := range env {
		if value, ok := os.LookupEnv(name); ok {
			if err := c.Set(key, value); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
	}
	return nil
}

// Set aplica una clave individual. Acepta nombres con '-' o '_'.
func (c *Config) Set(key, value string) error {
	key = strings.ReplaceAll(strings.ToLower(key), "-", "_")
	switch key {
	case "addr":
		c.Addr = value
	case "snapshot":
		c.SnapshotPath = value
	case "save_interval":
		d, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		c.SaveInterval = d
	case "aof":
		c.AOFPath = value
	case "aof_fsync":
		c.AOFSync = value
	case "requirepass":
		c.AuthPassword = value
	case "tls_cert":
		c.TLSCertPath = value
	case "tls_key":
		c.TLSKeyPath = value
	case "max_connections":
		n, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		c.MaxConnections = n
	case "read_timeout":
		d, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		c.ReadTimeout = d
	case "write_timeout":
		d, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		c.WriteTimeout = d
	case "max_memory":
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return err
		}
		c.MaxMemoryBytes = n
	case "slowlog_threshold":
		d, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		c.SlowLogThreshold = d
	case "slowlog_max_len":
		n, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		c.SlowLogMaxLen = n
	case "shutdown_timeout":
		d, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		c.ShutdownTimeout = d
	default:
		return fmt.Errorf("clave desconocida %q", key)
	}
	return nil
}

// MergeOverlay reemplaza solo campos con valor explícito en overlay.
func (c Config) MergeOverlay(overlay Config) Config {
	if overlay.Addr != "" {
		c.Addr = overlay.Addr
	}
	if overlay.SnapshotPath != "" {
		c.SnapshotPath = overlay.SnapshotPath
	}
	if overlay.SaveInterval != 0 {
		c.SaveInterval = overlay.SaveInterval
	}
	if overlay.AOFPath != "" {
		c.AOFPath = overlay.AOFPath
	}
	if overlay.AOFSync != "" {
		c.AOFSync = overlay.AOFSync
	}
	if overlay.AuthPassword != "" {
		c.AuthPassword = overlay.AuthPassword
	}
	if overlay.TLSCertPath != "" {
		c.TLSCertPath = overlay.TLSCertPath
	}
	if overlay.TLSKeyPath != "" {
		c.TLSKeyPath = overlay.TLSKeyPath
	}
	if overlay.MaxConnections != 0 {
		c.MaxConnections = overlay.MaxConnections
	}
	if overlay.ReadTimeout != 0 {
		c.ReadTimeout = overlay.ReadTimeout
	}
	if overlay.WriteTimeout != 0 {
		c.WriteTimeout = overlay.WriteTimeout
	}
	if overlay.MaxMemoryBytes != 0 {
		c.MaxMemoryBytes = overlay.MaxMemoryBytes
	}
	if overlay.SlowLogThreshold != 0 {
		c.SlowLogThreshold = overlay.SlowLogThreshold
	}
	if overlay.SlowLogMaxLen != 0 {
		c.SlowLogMaxLen = overlay.SlowLogMaxLen
	}
	if overlay.ShutdownTimeout != 0 {
		c.ShutdownTimeout = overlay.ShutdownTimeout
	}
	return c
}

// ServerOptions convierte Config a server.Options.
func (c Config) ServerOptions() server.Options {
	return server.Options{
		Addr:             c.Addr,
		SnapshotPath:     c.SnapshotPath,
		SaveInterval:     c.SaveInterval,
		AOFPath:          c.AOFPath,
		AOFSync:          c.AOFSync,
		AuthPassword:     c.AuthPassword,
		TLSCertPath:      c.TLSCertPath,
		TLSKeyPath:       c.TLSKeyPath,
		MaxConnections:   c.MaxConnections,
		ReadTimeout:      c.ReadTimeout,
		WriteTimeout:     c.WriteTimeout,
		MaxMemoryBytes:   c.MaxMemoryBytes,
		SlowLogThreshold: c.SlowLogThreshold,
		SlowLogMaxLen:    c.SlowLogMaxLen,
		ShutdownTimeout:  c.ShutdownTimeout,
	}
}
