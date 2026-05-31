# Llavero Fase 1 — Plan de Implementación (Esqueleto + Servidor TCP)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Construir el esqueleto del proyecto y un servidor TCP concurrente que responde `PING` con `PONG`, una goroutine por conexión, resistente a pánicos.

**Architecture:** Un paquete `internal/server` con un tipo `Server` que separa el ciclo de vida (`New`/`Listen`/`Addr`/`Serve`/`Close`) del manejo de cada conexión (`handleConn`). El binario en `cmd/llavero` solo cablea y arranca. Se usa solo la librería estándar.

**Tech Stack:** Go 1.26, librería estándar (`net`, `bufio`, `strings`, `fmt`, `log`, `testing`). Sin dependencias externas.

---

## Estructura de archivos

- `go.mod` — módulo `llavero`.
- `cmd/llavero/main.go` — punto de entrada; crea y arranca el `Server`.
- `internal/server/server.go` — tipo `Server`: ciclo de vida + `handleConn`.
- `internal/server/server_test.go` — tests de ciclo de vida y de conexión.

Decisión: en Fase 1 el manejo de comandos vive inline en `handleConn` (solo `PING`). La interfaz `Protocol` y el dispatcher se introducen en la Fase 2, cuando hay más de un comando real que justifique la abstracción (YAGNI).

---

### Task 1: Esqueleto del módulo

**Files:**
- Create: `go.mod` (vía comando)
- Create: directorios `cmd/llavero/`, `internal/server/`

- [ ] **Step 1: Inicializar el módulo Go**

Run:
```bash
go mod init llavero
mkdir -p cmd/llavero internal/server
```
Expected: se crea `go.mod` con `module llavero` y la línea `go 1.26`.

- [ ] **Step 2: Verificar que compila (aún sin código)**

Run: `go build ./...`
Expected: sin salida y código de salida 0 (no hay paquetes con código todavía).

- [ ] **Step 3: Commit**

```bash
git add go.mod
git commit -m "chore: inicializa módulo Go llavero"
```

---

### Task 2: Ciclo de vida del servidor (New/Listen/Addr/Close)

**Files:**
- Create: `internal/server/server.go`
- Test: `internal/server/server_test.go`

- [ ] **Step 1: Escribir el test que falla**

Crear `internal/server/server_test.go`:
```go
package server

import "testing"

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
```

- [ ] **Step 2: Ejecutar el test y verificar que falla**

Run: `go test ./internal/server/ -run TestServerListensOnEphemeralPort -v`
Expected: FALLA al compilar con "undefined: New" (el tipo y los métodos aún no existen).

- [ ] **Step 3: Implementación mínima**

Crear `internal/server/server.go`:
```go
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
```

- [ ] **Step 4: Ejecutar el test y verificar que pasa**

Run: `go test ./internal/server/ -run TestServerListensOnEphemeralPort -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/server/server.go internal/server/server_test.go
git commit -m "feat: ciclo de vida del servidor TCP (Listen/Addr/Close)"
```

---

### Task 3: PING → PONG (Serve + handleConn)

**Files:**
- Modify: `internal/server/server.go`
- Modify: `internal/server/server_test.go`

- [ ] **Step 1: Escribir el helper de test y el test que falla**

Añadir al inicio de `internal/server/server_test.go` (tras la línea `import "testing"`, reemplazando ese import por el bloque siguiente):
```go
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

func TestPingReturnsPong(t *testing.T) {
	addr := startTestServer(t)
	if got := sendCommand(t, addr, "PING"); got != "PONG" {
		t.Fatalf("esperaba PONG, obtuve %q", got)
	}
}
```

- [ ] **Step 2: Ejecutar el test y verificar que falla**

Run: `go test ./internal/server/ -run TestPingReturnsPong -v`
Expected: FALLA al compilar con "s.Serve undefined".

- [ ] **Step 3: Implementación mínima**

Añadir a `internal/server/server.go`. Primero ampliar el bloque de imports:
```go
import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
)
```
Y añadir los métodos `Serve` y `handleConn`:
```go
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
```

- [ ] **Step 4: Ejecutar el test y verificar que pasa**

Run: `go test ./internal/server/ -run TestPingReturnsPong -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/server/server.go internal/server/server_test.go
git commit -m "feat: responde PING con PONG, goroutine por conexión"
```

---

### Task 4: Error para comandos desconocidos

**Files:**
- Modify: `internal/server/server_test.go`

(La implementación ya existe en el `default` del switch; este task fija el comportamiento con un test.)

- [ ] **Step 1: Escribir el test que falla**

Añadir a `internal/server/server_test.go`:
```go
func TestUnknownCommandReturnsError(t *testing.T) {
	addr := startTestServer(t)
	got := sendCommand(t, addr, "NOEXISTE")
	if !strings.HasPrefix(got, "ERR") {
		t.Fatalf("esperaba respuesta que empiece con ERR, obtuve %q", got)
	}
}
```

- [ ] **Step 2: Ejecutar el test y verificar que pasa**

Run: `go test ./internal/server/ -run TestUnknownCommandReturnsError -v`
Expected: PASS (el `default` del switch ya cubre este caso). Si fallara, revisar el switch de `handleConn`.

- [ ] **Step 3: Commit**

```bash
git add internal/server/server_test.go
git commit -m "test: error en comando desconocido"
```

---

### Task 5: Conexiones concurrentes

**Files:**
- Modify: `internal/server/server_test.go`

- [ ] **Step 1: Escribir el test que falla**

Añadir a `internal/server/server_test.go`:
```go
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
```

- [ ] **Step 2: Ejecutar con el detector de carreras**

Run: `go test ./internal/server/ -run TestConcurrentConnections -race -v`
Expected: PASS, sin avisos del data race detector.

- [ ] **Step 3: Ejecutar toda la suite con -race**

Run: `go test ./... -race`
Expected: PASS en todos los paquetes.

- [ ] **Step 4: Commit**

```bash
git add internal/server/server_test.go
git commit -m "test: maneja múltiples conexiones concurrentes"
```

---

### Task 6: Punto de entrada del binario

**Files:**
- Create: `cmd/llavero/main.go`

- [ ] **Step 1: Escribir el `main`**

Crear `cmd/llavero/main.go`:
```go
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
```

- [ ] **Step 2: Compilar todo**

Run: `go build ./...`
Expected: sin errores; se puede generar el binario.

- [ ] **Step 3: Prueba de humo manual**

En una terminal:
```bash
go run ./cmd/llavero
```
Expected: log "Llavero escuchando en [::]:6380".

En otra terminal:
```bash
printf 'PING\n' | nc localhost 6380
```
Expected: imprime `PONG`. Detener el servidor con Ctrl-C.

- [ ] **Step 4: Commit**

```bash
git add cmd/llavero/main.go
git commit -m "feat: binario cmd/llavero que arranca el servidor"
```

---

## Verificación final de la fase

- [ ] `go vet ./...` sin avisos.
- [ ] `go test ./... -race` en verde.
- [ ] Prueba de humo manual con `nc` devuelve `PONG`.

Al terminar la Fase 1 tendremos: módulo Go, servidor TCP concurrente, `PING`/`PONG`, errores legibles y resistencia a pánicos. La Fase 2 (núcleo KV: protocolo propio + `GET`/`SET`/`DEL` + store con sharding) será un nuevo ciclo spec→plan.
