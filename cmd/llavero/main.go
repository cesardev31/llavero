package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"llavero/internal/server"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:6380", "dirección TCP de escucha")
	snapshot := flag.String("snapshot", "llavero.snapshot", "archivo de snapshot; vacío desactiva persistencia")
	saveInterval := flag.Duration("save-interval", 0, "intervalo de snapshot automático; 0 lo desactiva")
	aof := flag.String("aof", "", "archivo append-only; vacío desactiva AOF")
	aofSync := flag.String("aof-fsync", "always", "política fsync del AOF: always, everysec o no")
	requirePass := flag.String("requirepass", "", "contraseña requerida para AUTH; también puede venir de LLAVERO_REQUIREPASS")
	tlsCert := flag.String("tls-cert", "", "certificado TLS PEM; requiere -tls-key")
	tlsKey := flag.String("tls-key", "", "llave TLS PEM; requiere -tls-cert")
	maxConnections := flag.Int("max-connections", 0, "máximo de conexiones simultáneas; 0 lo desactiva")
	readTimeout := flag.Duration("read-timeout", 0, "timeout de lectura por comando; 0 lo desactiva")
	writeTimeout := flag.Duration("write-timeout", 0, "timeout de escritura por respuesta/pubsub; 0 lo desactiva")
	maxMemory := flag.Int64("max-memory", 0, "límite aproximado de memoria para claves/valores; 0 lo desactiva")
	flag.Parse()

	if *requirePass == "" {
		*requirePass = os.Getenv("LLAVERO_REQUIREPASS")
	}

	snapshotSet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "snapshot" {
			snapshotSet = true
		}
	})
	if *aof != "" && !snapshotSet {
		*snapshot = ""
	}

	s, err := server.NewWithOptions(server.Options{
		Addr:           *addr,
		SnapshotPath:   *snapshot,
		SaveInterval:   *saveInterval,
		AOFPath:        *aof,
		AOFSync:        *aofSync,
		AuthPassword:   *requirePass,
		TLSCertPath:    *tlsCert,
		TLSKeyPath:     *tlsKey,
		MaxConnections: *maxConnections,
		ReadTimeout:    *readTimeout,
		WriteTimeout:   *writeTimeout,
		MaxMemoryBytes: *maxMemory,
	})
	if err != nil {
		log.Fatalf("no se pudo cargar snapshot: %v", err)
	}
	if err := s.Listen(); err != nil {
		log.Fatalf("no se pudo escuchar: %v", err)
	}
	log.Printf("Llavero escuchando en %s", s.Addr())

	// apagado ordenado ante SIGINT/SIGTERM: guardar snapshot y cerrar.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("recibida señal %s, apagando...", sig)
		if err := s.Save(); err != nil {
			log.Printf("error al guardar snapshot final: %v", err)
		} else if *snapshot != "" {
			log.Printf("snapshot final guardado en %s", *snapshot)
		}
		_ = s.Close()
	}()

	if err := s.Serve(); err != nil {
		log.Fatalf("servidor detenido: %v", err)
	}
	log.Println("apagado limpio")
}
