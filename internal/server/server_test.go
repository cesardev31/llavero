package server

import (
	"bufio"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"llavero/internal/persistence"
	"llavero/internal/store"
)

func startTestServer(t *testing.T) string {
	t.Helper()
	s := New("127.0.0.1:0")
	if err := s.Listen(); err != nil {
		t.Fatalf("Listen devolvió error: %v", err)
	}
	go s.Serve()
	t.Cleanup(func() { s.Close() })
	return s.Addr()
}

// sendCommand envía las partes como una orden mini-RESP y devuelve la primera
// línea de respuesta (sin terminadores).
func sendCommand(t *testing.T, addr string, parts ...string) string {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Dial devolvió error: %v", err)
	}
	defer conn.Close()

	fmt.Fprintf(conn, "*%d\n", len(parts))
	for _, p := range parts {
		fmt.Fprintf(conn, "$%d\n%s\n", len(p), p)
	}
	reply, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("lectura devolvió error: %v", err)
	}
	return strings.TrimRight(reply, "\r\n")
}

func TestServerListensOnEphemeralPort(t *testing.T) {
	s := New("127.0.0.1:0")
	if err := s.Listen(); err != nil {
		t.Fatalf("Listen devolvió error: %v", err)
	}
	defer s.Close()
	if s.Addr() == "" {
		t.Fatal("Addr() está vacío tras Listen()")
	}
}

func TestPingReturnsPong(t *testing.T) {
	addr := startTestServer(t)
	if got := sendCommand(t, addr, "PING"); got != "+PONG" {
		t.Fatalf("esperaba +PONG, obtuve %q", got)
	}
}

func TestSetThenGet(t *testing.T) {
	addr := startTestServer(t)
	if got := sendCommand(t, addr, "SET", "saludo", "hola"); got != "+OK" {
		t.Fatalf("SET: esperaba +OK, obtuve %q", got)
	}
	if got := sendCommand(t, addr, "GET", "saludo"); got != "$4" {
		t.Fatalf("GET: esperaba línea bulk $4, obtuve %q", got)
	}
}

func TestUnknownCommandReturnsError(t *testing.T) {
	addr := startTestServer(t)
	got := sendCommand(t, addr, "NOEXISTE")
	if !strings.HasPrefix(got, "-ERR") {
		t.Fatalf("esperaba respuesta que empiece con -ERR, obtuve %q", got)
	}
}

// writeCmd serializa una orden mini-RESP en un writer ya conectado.
func writeCmd(w *bufio.Writer, parts ...string) {
	fmt.Fprintf(w, "*%d\n", len(parts))
	for _, p := range parts {
		fmt.Fprintf(w, "$%d\n%s\n", len(p), p)
	}
	w.Flush()
}

func TestGetReturnsFullValue(t *testing.T) {
	addr := startTestServer(t)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Dial devolvió error: %v", err)
	}
	defer conn.Close()
	w := bufio.NewWriter(conn)
	r := bufio.NewReader(conn)

	writeCmd(w, "SET", "k", "hola")
	if line, _ := r.ReadString('\n'); strings.TrimRight(line, "\r\n") != "+OK" {
		t.Fatalf("SET -> %q", line)
	}
	writeCmd(w, "GET", "k")
	hdr, _ := r.ReadString('\n')
	body, _ := r.ReadString('\n')
	if strings.TrimRight(hdr, "\r\n") != "$4" || strings.TrimRight(body, "\r\n") != "hola" {
		t.Fatalf("GET -> %q %q, quería $4 / hola", hdr, body)
	}
}

func TestListCommandsOverTCP(t *testing.T) {
	addr := startTestServer(t)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Dial devolvió error: %v", err)
	}
	defer conn.Close()
	w := bufio.NewWriter(conn)
	r := bufio.NewReader(conn)

	writeCmd(w, "RPUSH", "l", "a", "b")
	if line, _ := r.ReadString('\n'); strings.TrimRight(line, "\r\n") != ":2" {
		t.Fatalf("RPUSH -> %q", line)
	}
	writeCmd(w, "LRANGE", "l", "0", "-1")
	for _, want := range []string{"*2", "$1", "a", "$1", "b"} {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("leyendo LRANGE: %v", err)
		}
		if got := strings.TrimRight(line, "\r\n"); got != want {
			t.Fatalf("LRANGE -> %q, quería %q", got, want)
		}
	}
}

func TestSaveCommandPersistsSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dump.llavero")
	s, err := NewWithOptions(Options{Addr: "127.0.0.1:0", SnapshotPath: path})
	if err != nil {
		t.Fatalf("NewWithOptions devolvió error: %v", err)
	}
	if err := s.Listen(); err != nil {
		t.Fatalf("Listen devolvió error: %v", err)
	}
	go s.Serve()
	t.Cleanup(func() { s.Close() })
	addr := s.Addr()

	if got := sendCommand(t, addr, "SET", "k", "v"); got != "+OK" {
		t.Fatalf("SET -> %q", got)
	}
	if got := sendCommand(t, addr, "SAVE"); got != "+OK" {
		t.Fatalf("SAVE -> %q", got)
	}

	loaded := store.New(16)
	if err := persistence.Load(path, loaded); err != nil {
		t.Fatalf("Load snapshot -> %v", err)
	}
	if got, ok, err := loaded.Get("k"); err != nil || !ok || string(got) != "v" {
		t.Fatalf("snapshot GET -> %q %v %v", got, ok, err)
	}
}

func TestServerLoadsSnapshotAtStartup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dump.llavero")
	src := store.New(16)
	src.Set("k", []byte("v"))
	if err := persistence.Save(path, src); err != nil {
		t.Fatalf("Save fixture -> %v", err)
	}

	s, err := NewWithOptions(Options{Addr: "127.0.0.1:0", SnapshotPath: path})
	if err != nil {
		t.Fatalf("NewWithOptions devolvió error: %v", err)
	}
	if err := s.Listen(); err != nil {
		t.Fatalf("Listen devolvió error: %v", err)
	}
	go s.Serve()
	t.Cleanup(func() { s.Close() })

	conn, err := net.Dial("tcp", s.Addr())
	if err != nil {
		t.Fatalf("Dial devolvió error: %v", err)
	}
	defer conn.Close()
	w := bufio.NewWriter(conn)
	r := bufio.NewReader(conn)

	writeCmd(w, "GET", "k")
	hdr, _ := r.ReadString('\n')
	body, _ := r.ReadString('\n')
	if strings.TrimRight(hdr, "\r\n") != "$1" || strings.TrimRight(body, "\r\n") != "v" {
		t.Fatalf("GET cargado -> %q %q, quería $1 / v", hdr, body)
	}
}

func TestMalformedRequestGetsError(t *testing.T) {
	addr := startTestServer(t)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Dial devolvió error: %v", err)
	}
	defer conn.Close()

	fmt.Fprint(conn, "esto no es mini-resp\n")
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("lectura devolvió error: %v", err)
	}
	if !strings.HasPrefix(strings.TrimRight(line, "\r\n"), "-ERR") {
		t.Fatalf("esperaba -ERR ante petición malformada, obtuve %q", line)
	}
}

func TestConcurrentConnections(t *testing.T) {
	addr := startTestServer(t)
	const n = 10
	results := make(chan string, n)
	for i := 0; i < n; i++ {
		go func() {
			results <- sendCommand(t, addr, "PING")
		}()
	}
	for i := 0; i < n; i++ {
		if got := <-results; got != "+PONG" {
			t.Fatalf("conexión concurrente: esperaba +PONG, obtuve %q", got)
		}
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	s := New("127.0.0.1:0")
	if err := s.Listen(); err != nil {
		t.Fatalf("Listen devolvió error: %v", err)
	}
	go s.Serve()
	// cerrar dos veces no debe entrar en pánico: sync.Once protege el canal stop
	if err := s.Close(); err != nil {
		t.Fatalf("primer Close devolvió error: %v", err)
	}
	s.Close()
}

func TestServeReturnsNilOnGracefulClose(t *testing.T) {
	s := New("127.0.0.1:0")
	if err := s.Listen(); err != nil {
		t.Fatalf("Listen devolvió error: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- s.Serve() }()

	// dar un instante a que arranque el accept loop, luego cerrar
	time.Sleep(20 * time.Millisecond)
	s.Close()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve tras cierre ordenado devolvió %v, quería nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve no retornó tras Close")
	}
}

func TestSaveWritesSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dump.llavero")
	s, err := NewWithOptions(Options{Addr: "127.0.0.1:0", SnapshotPath: path})
	if err != nil {
		t.Fatalf("NewWithOptions devolvió error: %v", err)
	}
	s.store.Set("k", []byte("v"))

	if err := s.Save(); err != nil {
		t.Fatalf("Save devolvió error: %v", err)
	}

	loaded := store.New(16)
	if err := persistence.Load(path, loaded); err != nil {
		t.Fatalf("Load -> %v", err)
	}
	if got, ok, err := loaded.Get("k"); err != nil || !ok || string(got) != "v" {
		t.Fatalf("snapshot GET -> %q %v %v", got, ok, err)
	}
}

func TestSaveWithoutSnapshotPathIsNoop(t *testing.T) {
	s := New("127.0.0.1:0") // sin SnapshotPath
	s.store.Set("k", []byte("v"))
	if err := s.Save(); err != nil {
		t.Fatalf("Save sin snapshotPath debería ser no-op, devolvió %v", err)
	}
}
