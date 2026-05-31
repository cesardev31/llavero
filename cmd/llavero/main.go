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
	addr := flag.String("addr", ":6380", "dirección TCP de escucha")
	snapshot := flag.String("snapshot", "llavero.snapshot", "archivo de snapshot; vacío desactiva persistencia")
	saveInterval := flag.Duration("save-interval", 0, "intervalo de snapshot automático; 0 lo desactiva")
	aof := flag.String("aof", "", "archivo append-only; vacío desactiva AOF")
	aofSync := flag.String("aof-fsync", "always", "política fsync del AOF: always, everysec o no")
	flag.Parse()

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
		Addr:         *addr,
		SnapshotPath: *snapshot,
		SaveInterval: *saveInterval,
		AOFPath:      *aof,
		AOFSync:      *aofSync,
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
