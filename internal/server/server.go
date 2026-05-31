package server

import (
	"bufio"
	"io"
	"log"
	"net"

	"llavero/internal/command"
	"llavero/internal/protocol"
	"llavero/internal/store"
)

// Server es un servidor TCP de Llavero.
type Server struct {
	addr  string
	ln    net.Listener
	store *store.Store
	disp  *command.Dispatcher
	proto protocol.Protocol
}

// New crea un servidor que escuchará en la dirección dada (p.ej. ":6380").
func New(addr string) *Server {
	return &Server{
		addr:  addr,
		store: store.New(256),
		disp:  command.NewDispatcher(),
		proto: protocol.MiniRESP{},
	}
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
func (s *Server) Serve() error {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return err
		}
		go s.handleConn(conn)
	}
}

// handleConn atiende una conexión: parsea órdenes, las despacha y responde.
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
		cmd, err := s.proto.Parse(reader)
		if err != nil {
			if err == io.EOF {
				return // cliente cerró limpiamente entre órdenes
			}
			// error de protocolo: avisar al cliente y cerrar la conexión
			s.proto.Encode(conn, protocol.ErrorReply{Msg: "ERR " + err.Error()})
			return
		}
		reply := s.disp.Dispatch(s.store, cmd)
		if err := s.proto.Encode(conn, reply); err != nil {
			return
		}
	}
}
