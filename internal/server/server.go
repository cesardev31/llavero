package server

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
)

// Server es un servidor TCP de Llavero.
type Server struct {
	addr string
	ln   net.Listener
}

// New crea un servidor que escuchará en la dirección dada (p.ej. ":6380").
func New(addr string) *Server {
	return &Server{addr: addr}
}

// Listen abre el socket TCP. Debe llamarse antes de Serve.
func (s *Server) Listen() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.ln = ln
	return nil
}

// Addr devuelve la dirección real en la que escucha (útil con puerto :0).
func (s *Server) Addr() string {
	if s.ln == nil {
		return ""
	}
	return s.ln.Addr().String()
}

// Close cierra el socket de escucha.
func (s *Server) Close() error {
	if s.ln == nil {
		return nil
	}
	return s.ln.Close()
}

// Serve acepta conexiones y lanza una goroutine por cada una.
// Devuelve error cuando el listener se cierra.
func (s *Server) Serve() error {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return err
		}
		go s.handleConn(conn)
	}
}

// handleConn atiende una única conexión: lee líneas y responde comandos.
// Un pánico aquí solo afecta a esta conexión, nunca al servidor.
func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()
	defer func() {
		if r := recover(); r != nil {
			log.Printf("conexión %s recuperada de pánico: %v", conn.RemoteAddr(), r)
		}
	}()

	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return // cliente desconectado o error de lectura
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch cmd {
		case "":
			continue
		case "PING":
			fmt.Fprint(conn, "PONG\r\n")
		default:
			fmt.Fprintf(conn, "ERR comando desconocido: %s\r\n", cmd)
		}
	}
}
