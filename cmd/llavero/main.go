package main

import (
	"log"

	"llavero/internal/server"
)

func main() {
	s := server.New(":6380")
	if err := s.Listen(); err != nil {
		log.Fatalf("no se pudo escuchar: %v", err)
	}
	log.Printf("Llavero escuchando en %s", s.Addr())
	if err := s.Serve(); err != nil {
		log.Fatalf("servidor detenido: %v", err)
	}
}
