package server

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
)

// startTestServer arranca un servidor en un puerto efímero y devuelve su dirección.
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

// sendCommand abre una conexión, envía una línea y devuelve la respuesta (sin \r\n).
func sendCommand(t *testing.T, addr, cmd string) string {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Dial devolvió error: %v", err)
	}
	defer conn.Close()

	fmt.Fprintf(conn, "%s\n", cmd)
	reply, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("lectura devolvió error: %v", err)
	}
	return strings.TrimSpace(reply)
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
	if got := sendCommand(t, addr, "PING"); got != "PONG" {
		t.Fatalf("esperaba PONG, obtuve %q", got)
	}
}

func TestUnknownCommandReturnsError(t *testing.T) {
	addr := startTestServer(t)
	got := sendCommand(t, addr, "NOEXISTE")
	if !strings.HasPrefix(got, "ERR") {
		t.Fatalf("esperaba respuesta que empiece con ERR, obtuve %q", got)
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
		if got := <-results; got != "PONG" {
			t.Fatalf("conexión concurrente: esperaba PONG, obtuve %q", got)
		}
	}
}
