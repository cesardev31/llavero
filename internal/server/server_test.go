package server

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"llavero/internal/persistence"
	"llavero/internal/protocol"
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

func startTestServerWithOptions(t *testing.T, opts Options) (*Server, string) {
	t.Helper()
	if opts.Addr == "" {
		opts.Addr = "127.0.0.1:0"
	}
	s, err := NewWithOptions(opts)
	if err != nil {
		t.Fatalf("NewWithOptions devolvió error: %v", err)
	}
	if err := s.Listen(); err != nil {
		t.Fatalf("Listen devolvió error: %v", err)
	}
	go s.Serve()
	return s, s.Addr()
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

	fmt.Fprintf(conn, "*%d\r\n", len(parts))
	for _, p := range parts {
		fmt.Fprintf(conn, "$%d\r\n%s\r\n", len(p), p)
	}
	reply, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("lectura devolvió error: %v", err)
	}
	return strings.TrimRight(reply, "\r\n")
}

func sendBulkCommand(t *testing.T, addr string, parts ...string) string {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Dial devolvió error: %v", err)
	}
	defer conn.Close()
	w := bufio.NewWriter(conn)
	r := bufio.NewReader(conn)
	writeCmd(w, parts...)
	return readBulkString(t, r)
}

func readBulkString(t *testing.T, r *bufio.Reader) string {
	t.Helper()
	hdr, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("lectura bulk header -> %v", err)
	}
	hdr = strings.TrimRight(hdr, "\r\n")
	if !strings.HasPrefix(hdr, "$") {
		t.Fatalf("esperaba bulk, obtuve %q", hdr)
	}
	n, err := strconv.Atoi(hdr[1:])
	if err != nil {
		t.Fatalf("bulk inválido %q: %v", hdr, err)
	}
	if n < 0 {
		return ""
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		t.Fatalf("lectura bulk body -> %v", err)
	}
	if _, err := r.Discard(2); err != nil {
		t.Fatalf("discard crlf -> %v", err)
	}
	return string(buf)
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
	fmt.Fprintf(w, "*%d\r\n", len(parts))
	for _, p := range parts {
		fmt.Fprintf(w, "$%d\r\n%s\r\n", len(p), p)
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

func TestMaxConnectionsRejectsExtraClients(t *testing.T) {
	s, addr := startTestServerWithOptions(t, Options{MaxConnections: 1})
	t.Cleanup(func() { s.Close() })

	conn1, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Dial conn1 -> %v", err)
	}
	defer conn1.Close()

	conn2, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Dial conn2 -> %v", err)
	}
	defer conn2.Close()
	if err := conn2.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline -> %v", err)
	}
	line, err := bufio.NewReader(conn2).ReadString('\n')
	if err != nil {
		t.Fatalf("lectura conn2 -> %v", err)
	}
	if got := strings.TrimRight(line, "\r\n"); got != "-ERR max clients reached" {
		t.Fatalf("conn2 -> %q", got)
	}
}

func TestReadTimeoutClosesIdleClient(t *testing.T) {
	s, addr := startTestServerWithOptions(t, Options{ReadTimeout: 20 * time.Millisecond})
	t.Cleanup(func() { s.Close() })

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Dial -> %v", err)
	}
	defer conn.Close()
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline -> %v", err)
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("lectura timeout -> %v", err)
	}
	if !strings.HasPrefix(strings.TrimRight(line, "\r\n"), "-ERR") {
		t.Fatalf("timeout -> %q", line)
	}
}

func TestClientSendSetsWriteDeadline(t *testing.T) {
	conn := &deadlineConn{}
	c := newClient(conn, protocol.RESP{}, 50*time.Millisecond)
	if err := c.send(protocol.StatusReply{Msg: "OK"}); err != nil {
		t.Fatalf("send -> %v", err)
	}
	if !conn.writeDeadlineSet {
		t.Fatal("send no fijó write deadline")
	}
	if got := conn.String(); got != "+OK\r\n" {
		t.Fatalf("payload -> %q", got)
	}
}

func TestMaxMemoryRejectsGrowingWrites(t *testing.T) {
	s, addr := startTestServerWithOptions(t, Options{MaxMemoryBytes: 4})
	t.Cleanup(func() { s.Close() })

	if got := sendCommand(t, addr, "SET", "k", "v"); got != "+OK" {
		t.Fatalf("SET pequeño -> %q", got)
	}
	if got := sendCommand(t, addr, "SET", "big", "12345"); got != "-OOM command not allowed when used memory > maxmemory" {
		t.Fatalf("SET grande -> %q", got)
	}
	if got := sendCommand(t, addr, "GET", "big"); got != "$-1" {
		t.Fatalf("GET big tras OOM -> %q", got)
	}
}

func TestResourceLimitOptionsRejectNegativeValues(t *testing.T) {
	if _, err := NewWithOptions(Options{MaxConnections: -1}); err == nil {
		t.Fatal("MaxConnections negativo fue aceptado")
	}
	if _, err := NewWithOptions(Options{MaxMemoryBytes: -1}); err == nil {
		t.Fatal("MaxMemoryBytes negativo fue aceptado")
	}
}

func TestInfoAndStatsExposeMetrics(t *testing.T) {
	s, addr := startTestServerWithOptions(t, Options{MaxConnections: 2, MaxMemoryBytes: 1024})
	t.Cleanup(func() { s.Close() })

	if got := sendCommand(t, addr, "PING"); got != "+PONG" {
		t.Fatalf("PING -> %q", got)
	}
	if got := sendCommand(t, addr, "SET", "obs", "ok"); got != "+OK" {
		t.Fatalf("SET -> %q", got)
	}
	info := sendBulkCommand(t, addr, "INFO")
	for _, want := range []string{
		"# Server",
		"connected_clients:",
		"total_commands_processed:",
		"used_memory_approx:",
		"maxmemory:1024",
		"cmdstat_ping:calls=1",
		"cmdstat_set:calls=1",
	} {
		if !strings.Contains(info, want) {
			t.Fatalf("INFO no contiene %q:\n%s", want, info)
		}
	}
	stats := sendBulkCommand(t, addr, "STATS")
	if !strings.Contains(stats, "total_commands_processed:") {
		t.Fatalf("STATS inesperado:\n%s", stats)
	}
}

func TestInfoTracksRejectedConnections(t *testing.T) {
	s, addr := startTestServerWithOptions(t, Options{MaxConnections: 1})
	t.Cleanup(func() { s.Close() })

	conn1, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Dial conn1 -> %v", err)
	}
	defer conn1.Close()
	time.Sleep(20 * time.Millisecond)
	conn2, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Dial conn2 -> %v", err)
	}
	if err := conn2.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline -> %v", err)
	}
	if _, err := bufio.NewReader(conn2).ReadString('\n'); err != nil {
		t.Fatalf("lectura conn2 -> %v", err)
	}
	_ = conn2.Close()
	_ = conn1.Close()

	time.Sleep(20 * time.Millisecond)
	info := sendBulkCommand(t, addr, "INFO")
	if !strings.Contains(info, "rejected_connections:1") {
		t.Fatalf("INFO no registró rechazo:\n%s", info)
	}
}

func TestSlowLogLenGetAndReset(t *testing.T) {
	s, addr := startTestServerWithOptions(t, Options{
		SlowLogThreshold: time.Nanosecond,
		SlowLogMaxLen:    4,
	})
	t.Cleanup(func() { s.Close() })

	if got := sendCommand(t, addr, "PING"); got != "+PONG" {
		t.Fatalf("PING -> %q", got)
	}
	if got := sendCommand(t, addr, "SLOWLOG", "LEN"); !strings.HasPrefix(got, ":") || got == ":0" {
		t.Fatalf("SLOWLOG LEN -> %q", got)
	}
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Dial -> %v", err)
	}
	defer conn.Close()
	w := bufio.NewWriter(conn)
	r := bufio.NewReader(conn)
	writeCmd(w, "SLOWLOG", "GET", "1")
	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("SLOWLOG GET lectura -> %v", err)
	}
	if got := strings.TrimRight(line, "\r\n"); got != "*1" {
		t.Fatalf("SLOWLOG GET header -> %q", got)
	}
	if got := sendCommand(t, addr, "SLOWLOG", "RESET"); got != "+OK" {
		t.Fatalf("SLOWLOG RESET -> %q", got)
	}
	if got := sendCommand(t, addr, "SLOWLOG", "LEN"); got != ":0" {
		t.Fatalf("SLOWLOG LEN tras reset -> %q", got)
	}
}

func TestObservabilityOptionsRejectNegativeValues(t *testing.T) {
	if _, err := NewWithOptions(Options{SlowLogThreshold: -time.Nanosecond}); err == nil {
		t.Fatal("SlowLogThreshold negativo fue aceptado")
	}
	if _, err := NewWithOptions(Options{SlowLogMaxLen: -1}); err == nil {
		t.Fatal("SlowLogMaxLen negativo fue aceptado")
	}
}

type deadlineConn struct {
	bytes.Buffer
	writeDeadlineSet bool
}

func (c *deadlineConn) Read([]byte) (int, error)        { return 0, io.EOF }
func (c *deadlineConn) Close() error                    { return nil }
func (c *deadlineConn) LocalAddr() net.Addr             { return testAddr("local") }
func (c *deadlineConn) RemoteAddr() net.Addr            { return testAddr("remote") }
func (c *deadlineConn) SetDeadline(time.Time) error     { return nil }
func (c *deadlineConn) SetReadDeadline(time.Time) error { return nil }
func (c *deadlineConn) SetWriteDeadline(t time.Time) error {
	if !t.IsZero() {
		c.writeDeadlineSet = true
	}
	return nil
}

type testAddr string

func (a testAddr) Network() string { return string(a) }
func (a testAddr) String() string  { return string(a) }

func TestAuthRequiresPasswordBeforeCommands(t *testing.T) {
	s, addr := startTestServerWithOptions(t, Options{AuthPassword: "secreto"})
	t.Cleanup(func() { s.Close() })

	if got := sendCommand(t, addr, "GET", "k"); got != "-NOAUTH Authentication required." {
		t.Fatalf("GET sin AUTH -> %q", got)
	}
	if got := sendCommand(t, addr, "AUTH", "mal"); got != "-ERR contraseña inválida" {
		t.Fatalf("AUTH mal -> %q", got)
	}

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Dial -> %v", err)
	}
	defer conn.Close()
	w := bufio.NewWriter(conn)
	r := bufio.NewReader(conn)

	writeCmd(w, "AUTH", "secreto")
	if got := readReply(t, r); len(got) != 1 || got[0] != "OK" {
		t.Fatalf("AUTH correcto -> %v", got)
	}
	writeCmd(w, "SET", "k", "v")
	if got := readReply(t, r); len(got) != 1 || got[0] != "OK" {
		t.Fatalf("SET autenticado -> %v", got)
	}
	writeCmd(w, "GET", "k")
	hdr, _ := r.ReadString('\n')
	body, _ := r.ReadString('\n')
	if strings.TrimRight(hdr, "\r\n") != "$1" || strings.TrimRight(body, "\r\n") != "v" {
		t.Fatalf("GET autenticado -> %q %q", hdr, body)
	}
}

func TestAuthNotRequiredByDefault(t *testing.T) {
	addr := startTestServer(t)
	if got := sendCommand(t, addr, "AUTH", "anything"); got != "-ERR AUTH no requerido" {
		t.Fatalf("AUTH sin password configurado -> %q", got)
	}
	if got := sendCommand(t, addr, "PING"); got != "+PONG" {
		t.Fatalf("PING sin AUTH -> %q", got)
	}
}

func TestListenRejectsPartialTLSConfig(t *testing.T) {
	s, err := NewWithOptions(Options{Addr: "127.0.0.1:0", TLSCertPath: "cert.pem"})
	if err != nil {
		t.Fatalf("NewWithOptions -> %v", err)
	}
	if err := s.Listen(); err == nil {
		t.Fatal("Listen aceptó TLS sin key")
	}
}

func TestTLSListener(t *testing.T) {
	certPath, keyPath := writeTestCertificate(t)
	s, addr := startTestServerWithOptions(t, Options{
		TLSCertPath: certPath,
		TLSKeyPath:  keyPath,
	})
	t.Cleanup(func() { s.Close() })

	conn, err := tls.Dial("tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	})
	if err != nil {
		t.Fatalf("TLS Dial -> %v", err)
	}
	defer conn.Close()

	w := bufio.NewWriter(conn)
	r := bufio.NewReader(conn)
	writeCmd(w, "PING")
	if got := readReply(t, r); len(got) != 1 || got[0] != "PONG" {
		t.Fatalf("PING TLS -> %v", got)
	}
}

func writeTestCertificate(t *testing.T) (string, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey -> %v", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate -> %v", err)
	}
	dir := t.TempDir()
	certPath := filepath.Join(dir, "server.crt")
	keyPath := filepath.Join(dir, "server.key")
	certFile, err := os.Create(certPath)
	if err != nil {
		t.Fatalf("Create cert -> %v", err)
	}
	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatalf("Encode cert -> %v", err)
	}
	if err := certFile.Close(); err != nil {
		t.Fatalf("Close cert -> %v", err)
	}
	keyFile, err := os.Create(keyPath)
	if err != nil {
		t.Fatalf("Create key -> %v", err)
	}
	if err := pem.Encode(keyFile, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}); err != nil {
		t.Fatalf("Encode key -> %v", err)
	}
	if err := keyFile.Close(); err != nil {
		t.Fatalf("Close key -> %v", err)
	}
	return certPath, keyPath
}

func TestAOFRecoversWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "appendonly.aof")
	s, addr := startTestServerWithOptions(t, Options{AOFPath: path, AOFSync: "always"})

	if got := sendCommand(t, addr, "SET", "k", "v"); got != "+OK" {
		t.Fatalf("SET -> %q", got)
	}
	if got := sendCommand(t, addr, "RPUSH", "lista", "a", "b"); got != ":2" {
		t.Fatalf("RPUSH -> %q", got)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close primer servidor -> %v", err)
	}

	s2, addr2 := startTestServerWithOptions(t, Options{AOFPath: path, AOFSync: "always"})
	t.Cleanup(func() { s2.Close() })
	if got := sendCommand(t, addr2, "GET", "k"); got != "$1" {
		t.Fatalf("GET recuperado header -> %q, quería $1", got)
	}

	conn, err := net.Dial("tcp", addr2)
	if err != nil {
		t.Fatalf("Dial -> %v", err)
	}
	defer conn.Close()
	w := bufio.NewWriter(conn)
	r := bufio.NewReader(conn)
	writeCmd(w, "LRANGE", "lista", "0", "-1")
	if got := readReply(t, r); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("LRANGE recuperado -> %v", got)
	}
}

func TestAOFRecoversAbsoluteTTL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "appendonly.aof")
	s, addr := startTestServerWithOptions(t, Options{AOFPath: path, AOFSync: "always"})
	if got := sendCommand(t, addr, "SET", "temp", "v"); got != "+OK" {
		t.Fatalf("SET -> %q", got)
	}
	if got := sendCommand(t, addr, "EXPIRE", "temp", "1"); got != ":1" {
		t.Fatalf("EXPIRE -> %q", got)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close primer servidor -> %v", err)
	}

	time.Sleep(1100 * time.Millisecond)

	s2, addr2 := startTestServerWithOptions(t, Options{AOFPath: path, AOFSync: "always"})
	t.Cleanup(func() { s2.Close() })
	if got := sendCommand(t, addr2, "GET", "temp"); got != "$-1" {
		t.Fatalf("GET temp tras replay tardío -> %q, quería $-1", got)
	}
}

func TestAOFDoesNotRecordFailedMutations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "appendonly.aof")
	s, addr := startTestServerWithOptions(t, Options{AOFPath: path, AOFSync: "always"})
	if got := sendCommand(t, addr, "SET", "str", "v"); got != "+OK" {
		t.Fatalf("SET -> %q", got)
	}
	if got := sendCommand(t, addr, "RPUSH", "str", "x"); !strings.HasPrefix(got, "-WRONGTYPE") {
		t.Fatalf("RPUSH wrongtype -> %q", got)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close -> %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile AOF -> %v", err)
	}
	if strings.Contains(string(data), "RPUSH") {
		t.Fatalf("AOF registró comando fallido: %q", data)
	}
}

func TestAOFAndSnapshotAreMutuallyExclusive(t *testing.T) {
	_, err := NewWithOptions(Options{
		Addr:         "127.0.0.1:0",
		SnapshotPath: filepath.Join(t.TempDir(), "dump.llavero"),
		AOFPath:      filepath.Join(t.TempDir(), "appendonly.aof"),
	})
	if err == nil {
		t.Fatal("NewWithOptions aceptó snapshot y AOF juntos")
	}
}

// readReply lee una respuesta RESP simple o de array y devuelve sus elementos
// como strings (bulk e int). Suficiente para los asserts de pub/sub.
func readReply(t *testing.T, r *bufio.Reader) []string {
	t.Helper()
	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("lectura: %v", err)
	}
	line = strings.TrimRight(line, "\r\n")
	if len(line) == 0 {
		t.Fatal("respuesta vacía")
	}
	if line[0] == '*' {
		n, _ := strconv.Atoi(line[1:])
		out := make([]string, n)
		for i := 0; i < n; i++ {
			out[i] = readReplyScalar(t, r)
		}
		return out
	}
	return []string{line[1:]} // :int, +status, -error → strip the type prefix
}

func readReplyScalar(t *testing.T, r *bufio.Reader) string {
	t.Helper()
	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("lectura: %v", err)
	}
	line = strings.TrimRight(line, "\r\n")
	if line[0] == '$' {
		n, _ := strconv.Atoi(line[1:])
		if n < 0 {
			return ""
		}
		buf := make([]byte, n)
		if _, err := io.ReadFull(r, buf); err != nil {
			t.Fatalf("lectura bulk: %v", err)
		}
		if _, err := r.Discard(2); err != nil {
			t.Fatalf("discard crlf: %v", err)
		}
		return string(buf)
	}
	return line[1:] // :int, +status, -error
}

func TestPubSub(t *testing.T) {
	addr := startTestServer(t)

	subConn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Dial sub: %v", err)
	}
	defer subConn.Close()
	subR := bufio.NewReader(subConn)
	subW := bufio.NewWriter(subConn)

	writeCmd(subW, "SUBSCRIBE", "news")
	if got := readReply(t, subR); len(got) != 3 || got[0] != "subscribe" || got[1] != "news" || got[2] != "1" {
		t.Fatalf("subscribe reply → %v", got)
	}

	pubConn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Dial pub: %v", err)
	}
	defer pubConn.Close()
	pubR := bufio.NewReader(pubConn)
	pubW := bufio.NewWriter(pubConn)

	writeCmd(pubW, "PUBLISH", "news", "hola")
	if got := readReply(t, pubR); len(got) != 1 || got[0] != "1" {
		t.Fatalf("PUBLISH → %v, quería 1 receptor", got)
	}

	if msg := readReply(t, subR); len(msg) != 3 || msg[0] != "message" || msg[1] != "news" || msg[2] != "hola" {
		t.Fatalf("mensaje recibido → %v", msg)
	}
}
