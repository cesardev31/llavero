package main

import (
	"flag"
	"log"

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
	if err := s.Serve(); err != nil {
		log.Fatalf("servidor detenido: %v", err)
	}
}
