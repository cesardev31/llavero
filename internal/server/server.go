package server

import (
	"bufio"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"llavero/internal/command"
	"llavero/internal/protocol"
	"llavero/internal/store"
)

// expireInterval es cada cuánto corre la expiración activa (como el cron de Redis).
const expireInterval = 100 * time.Millisecond

// Server es un servidor TCP de Llavero.
type Server struct {
	addr      string
	ln        net.Listener
	store     *store.Store
	disp      *command.Dispatcher
	proto     protocol.Protocol
	stop      chan struct{}
	closeOnce sync.Once
}

// New crea un servidor que escuchará en la dirección dada (p.ej. ":6380").
func New(addr string) *Server {
	return &Server{
		addr:  addr,
		store: store.New(256),
		disp:  command.NewDispatcher(),
		proto: protocol.MiniRESP{},
		stop:  make(chan struct{}),
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

// Close detiene la expiración activa y cierra el socket. Es seguro llamarlo
// varias veces (sync.Once protege el cierre del canal stop).
func (s *Server) Close() error {
	s.closeOnce.Do(func() { close(s.stop) })
	if s.ln == nil {
		return nil
	}
	return s.ln.Close()
}

// Serve lanza la expiración activa y acepta conexiones (una goroutine por una).
func (s *Server) Serve() error {
	go s.expireLoop()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return err
		}
		go s.handleConn(conn)
	}
}

// expireLoop ejecuta la expiración activa periódicamente hasta el cierre.
func (s *Server) expireLoop() {
	t := time.NewTicker(expireInterval)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			s.store.ActiveExpireCycle()
		}
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
			_ = s.proto.Encode(conn, protocol.ErrorReply{Msg: "ERR " + err.Error()})
			return
		}
		reply := s.disp.Dispatch(s.store, cmd)
		if err := s.proto.Encode(conn, reply); err != nil {
			return
		}
	}
}
