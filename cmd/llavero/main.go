package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"llavero/internal/config"
	"llavero/internal/server"
)

func main() {
	var cfgPath string
	flags := config.Default()
	flag.StringVar(&cfgPath, "config", "", "archivo de configuración key=value")
	flag.StringVar(&flags.Addr, "addr", flags.Addr, "dirección TCP de escucha")
	flag.StringVar(&flags.SnapshotPath, "snapshot", flags.SnapshotPath, "archivo de snapshot; vacío desactiva persistencia")
	flag.DurationVar(&flags.SaveInterval, "save-interval", 0, "intervalo de snapshot automático; 0 lo desactiva")
	flag.StringVar(&flags.AOFPath, "aof", "", "archivo append-only; vacío desactiva AOF")
	flag.StringVar(&flags.AOFSync, "aof-fsync", flags.AOFSync, "política fsync del AOF: always, everysec o no")
	flag.StringVar(&flags.AuthPassword, "requirepass", "", "contraseña requerida para AUTH; también puede venir de LLAVERO_REQUIREPASS")
	flag.StringVar(&flags.TLSCertPath, "tls-cert", "", "certificado TLS PEM; requiere -tls-key")
	flag.StringVar(&flags.TLSKeyPath, "tls-key", "", "llave TLS PEM; requiere -tls-cert")
	flag.IntVar(&flags.MaxConnections, "max-connections", 0, "máximo de conexiones simultáneas; 0 lo desactiva")
	flag.DurationVar(&flags.ReadTimeout, "read-timeout", 0, "timeout de lectura por comando; 0 lo desactiva")
	flag.DurationVar(&flags.WriteTimeout, "write-timeout", 0, "timeout de escritura por respuesta/pubsub; 0 lo desactiva")
	flag.Int64Var(&flags.MaxMemoryBytes, "max-memory", 0, "límite aproximado de memoria para claves/valores; 0 lo desactiva")
	flag.DurationVar(&flags.SlowLogThreshold, "slowlog-threshold", 0, "latencia mínima para registrar en SLOWLOG; 0 lo desactiva")
	flag.IntVar(&flags.SlowLogMaxLen, "slowlog-max-len", flags.SlowLogMaxLen, "máximo de entradas retenidas en SLOWLOG")
	flag.DurationVar(&flags.ShutdownTimeout, "shutdown-timeout", flags.ShutdownTimeout, "tiempo máximo para drenar conexiones durante apagado")
	flag.Parse()

	visited := visitedFlags()
	cfg := config.Default()
	if cfgPath != "" {
		if err := cfg.ApplyFile(cfgPath); err != nil {
			log.Fatalf("no se pudo cargar config: %v", err)
		}
	}
	if err := cfg.ApplyEnv(); err != nil {
		log.Fatalf("configuración de entorno inválida: %v", err)
	}
	cfg = mergeVisitedFlags(cfg, flags, visited)

	// Compatibilidad con el default histórico: si se activa AOF y no se pasó
	// snapshot explícito, desactivar el snapshot por defecto.
	if cfg.AOFPath != "" && !visited["snapshot"] {
		cfg.SnapshotPath = ""
	}

	s, err := server.NewWithOptions(cfg.ServerOptions())
	if err != nil {
		log.Fatalf("no se pudo crear servidor: %v", err)
	}
	if err := s.Listen(); err != nil {
		log.Fatalf("no se pudo escuchar: %v", err)
	}
	log.Printf("event=start addr=%q", s.Addr())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("event=shutdown_signal signal=%q", sig)
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
	setDuration("slowlog-threshold", &base.SlowLogThreshold, flags.SlowLogThreshold)
	setInt("slowlog-max-len", &base.SlowLogMaxLen, flags.SlowLogMaxLen)
	setDuration("shutdown-timeout", &base.ShutdownTimeout, flags.ShutdownTimeout)
	return base
}
