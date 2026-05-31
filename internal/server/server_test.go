package server

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
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
