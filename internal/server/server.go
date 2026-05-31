package server

import (
	"bufio"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"llavero/internal/command"
	"llavero/internal/persistence"
	"llavero/internal/protocol"
	"llavero/internal/pubsub"
	"llavero/internal/store"
)

// expireInterval es cada cuánto corre la expiración activa (como el cron de Redis).
const expireInterval = 100 * time.Millisecond

// Server es un servidor TCP de Llavero.
type Server struct {
	addr         string
	ln           net.Listener
	store        *store.Store
	disp         *command.Dispatcher
	proto        protocol.Protocol
	broker       *pubsub.Broker
	stop         chan struct{}
	closeOnce    sync.Once
	snapshotPath string
	saveInterval time.Duration
}

// Options configura el servidor.
type Options struct {
	Addr         string
	SnapshotPath string
	SaveInterval time.Duration
}

// New crea un servidor que escuchará en la dirección dada (p.ej. ":6380").
func New(addr string) *Server {
	s, err := NewWithOptions(Options{Addr: addr})
	if err != nil {
		panic(err)
	}
	return s
}

// NewWithOptions crea un servidor y carga el snapshot si SnapshotPath existe.
func NewWithOptions(opts Options) (*Server, error) {
	if opts.Addr == "" {
		opts.Addr = ":6380"
	}
	st := store.New(256)
	if opts.SnapshotPath != "" {
		if err := persistence.Load(opts.SnapshotPath, st); err != nil {
			return nil, err
		}
	}
	disp := command.NewDispatcher()
	if opts.SnapshotPath != "" {
		disp = command.NewDispatcherWithSave(func(s *store.Store) error {
			return persistence.Save(opts.SnapshotPath, s)
		})
	}
	return &Server{
		addr:         opts.Addr,
		store:        st,
		disp:         disp,
		proto:        protocol.RESP{},
		broker:       pubsub.New(),
		stop:         make(chan struct{}),
		snapshotPath: opts.SnapshotPath,
		saveInterval: opts.SaveInterval,
	}, nil
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

// Save guarda un snapshot del store si hay snapshotPath configurado.
// Es no-op (sin error) si no se configuró persistencia.
func (s *Server) Save() error {
	if s.snapshotPath == "" {
		return nil
	}
	return persistence.Save(s.snapshotPath, s.store)
}

// Serve lanza la expiración activa y acepta conexiones (una goroutine por una).
func (s *Server) Serve() error {
	go s.expireLoop()
	if s.snapshotPath != "" && s.saveInterval > 0 {
		go s.saveLoop()
	}
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			// si el cierre fue ordenado (stop cerrado), no es un error
			select {
			case <-s.stop:
				return nil
			default:
				return err
			}
		}
		go s.handleConn(conn)
	}
}

// saveLoop guarda snapshots periódicos hasta el cierre.
func (s *Server) saveLoop() {
	t := time.NewTicker(s.saveInterval)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			if err := persistence.Save(s.snapshotPath, s.store); err != nil {
				log.Printf("no se pudo guardar snapshot: %v", err)
			}
		}
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
	c := newClient(conn, s.proto)
	defer s.unsubscribeAll(c)
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
			_ = c.send(protocol.ErrorReply{Msg: "ERR " + err.Error()})
			return
		}
		reply := s.handleCommand(c, cmd)
		if reply == nil {
			continue // el handler ya envió sus respuestas (p.ej. SUBSCRIBE)
		}
		if err := c.send(reply); err != nil {
			return
		}
	}
}

// handleCommand enruta los comandos con estado de conexión (pub/sub) al
// servidor y el resto al dispatcher.
func (s *Server) handleCommand(c *client, cmd protocol.Command) protocol.Reply {
	switch strings.ToUpper(cmd.Name) {
	case "SUBSCRIBE":
		return s.cmdSubscribe(c, cmd.Args)
	case "UNSUBSCRIBE":
		return s.cmdUnsubscribe(c, cmd.Args)
	case "PUBLISH":
		return s.cmdPublish(cmd.Args)
	default:
		return s.disp.Dispatch(s.store, cmd)
	}
}
