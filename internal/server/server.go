package server

import "net"

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
