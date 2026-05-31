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
	flag.Parse()

	s, err := server.NewWithOptions(server.Options{
		Addr:         *addr,
		SnapshotPath: *snapshot,
		SaveInterval: *saveInterval,
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
