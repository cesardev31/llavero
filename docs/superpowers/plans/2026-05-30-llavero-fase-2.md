# Llavero Fase 2 — Plan de Implementación (Núcleo KV)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Convertir el servidor de eco en un almacén clave-valor real: protocolo propio con prefijo de longitud (mini-RESP), almacén con sharding, y comandos `GET`/`SET`/`DEL`/`EXISTS` (más `PING`).

**Architecture:** Tres paquetes nuevos con responsabilidades aisladas: `internal/store` (almacén sharded thread-safe), `internal/protocol` (interfaz `Protocol` + implementación mini-RESP que parsea peticiones y serializa respuestas), `internal/command` (dispatcher que mapea nombre→handler). `internal/server` se reconecta para usar `Protocol.Parse → Dispatcher → Store → Protocol.Encode`.

**Tech Stack:** Go 1.26, librería estándar (`net`, `bufio`, `io`, `hash/fnv`, `sync`, `strconv`, `fmt`). Herramienta de desarrollo: Air (`github.com/air-verse/air`) para recarga en caliente.

---

## Formato del protocolo mini-RESP

Terminador de línea `\n`.

**Petición (cliente → servidor):**
```
*<N>\n               N = número de partes (comando + argumentos)
$<len>\n<bytes>\n    repetido N veces
```
Ejemplo `SET nombre cesar`: `*3\n$3\nSET\n$6\nnombre\n$5\ncesar\n`

**Respuesta (servidor → cliente):**
| Tipo | Formato | Uso |
|---|---|---|
| Estado | `+OK\n` | `SET` |
| Error | `-ERR mensaje\n` | comando inválido |
| Bulk | `$<len>\n<bytes>\n` | valor de `GET` |
| Nulo | `$-1\n` | `GET` de clave inexistente |
| Entero | `:<n>\n` | `DEL` (nº borradas), `EXISTS` (0/1) |

## Estructura de archivos

- `.air.toml` — config de recarga en caliente.
- `.gitignore` — ignora `tmp/` (binarios de Air).
- `internal/store/store.go` + `store_test.go` — almacén sharded.
- `internal/protocol/protocol.go` — interfaz `Protocol`, `Command`, tipos `Reply`.
- `internal/protocol/miniresp.go` + `miniresp_test.go` — implementación mini-RESP.
- `internal/command/command.go` — `Dispatcher` y registro de handlers.
- `internal/command/handlers.go` — handlers `cmdPing`/`cmdGet`/`cmdSet`/`cmdDel`/`cmdExists`.
- `internal/command/command_test.go` — tests del dispatcher y handlers.
- `internal/server/server.go` — reconexión a protocolo+dispatcher+store (reescritura de `handleConn` y `New`).
- `internal/server/server_test.go` — tests de integración actualizados a mini-RESP.

---

### Task 1: Herramienta de desarrollo (Air) + .gitignore

**Files:**
- Create: `.air.toml`
- Create: `.gitignore`

- [ ] **Step 1: Crear `.air.toml`**

```toml
root = "."
tmp_dir = "tmp"

[build]
  cmd = "go build -o ./tmp/llavero ./cmd/llavero"
  bin = "./tmp/llavero"
  delay = 1000
  include_ext = ["go"]
  exclude_dir = ["tmp", "docs", ".git"]
  exclude_regex = ["_test.go"]
  stop_on_error = true

[misc]
  clean_on_exit = true
```

- [ ] **Step 2: Crear `.gitignore`**

```
/tmp/
```

- [ ] **Step 3: Verificar que el build que usará Air funciona**

Run: `go build -o ./tmp/llavero ./cmd/llavero && echo OK && rm -rf tmp`
Expected: imprime `OK` (Air ejecutará ese mismo comando al recargar).

Nota: para usar la recarga en caliente, instalar Air una vez con
`go install github.com/air-verse/air@latest` y luego ejecutar `air` en la raíz.
Si Air no está instalado, `go run ./cmd/llavero` sigue funcionando.

- [ ] **Step 4: Commit**

```bash
git add .air.toml .gitignore
git commit -m "chore: config de Air para recarga en caliente"
```

---

### Task 2: Almacén con sharding (`internal/store`)

**Files:**
- Create: `internal/store/store.go`
- Test: `internal/store/store_test.go`

- [ ] **Step 1: Escribir los tests que fallan**

Crear `internal/store/store_test.go`:
```go
package store

import (
	"fmt"
	"sync"
	"testing"
)

func TestSetThenGet(t *testing.T) {
	s := New(16)
	s.Set("k", []byte("v"))
	got, ok := s.Get("k")
	if !ok {
		t.Fatal("Get no encontró la clave recién puesta")
	}
	if string(got) != "v" {
		t.Fatalf("Get = %q, quería v", got)
	}
}

func TestGetMissing(t *testing.T) {
	s := New(16)
	if _, ok := s.Get("nope"); ok {
		t.Fatal("Get devolvió ok=true para clave inexistente")
	}
}

func TestDelAndExists(t *testing.T) {
	s := New(16)
	s.Set("k", []byte("v"))
	if !s.Exists("k") {
		t.Fatal("Exists = false para clave existente")
	}
	if !s.Del("k") {
		t.Fatal("Del = false al borrar clave existente")
	}
	if s.Del("k") {
		t.Fatal("Del = true al borrar clave ya borrada")
	}
	if s.Exists("k") {
		t.Fatal("Exists = true tras borrar")
	}
}

func TestNextPow2(t *testing.T) {
	cases := map[int]int{0: 1, 1: 1, 2: 2, 3: 4, 5: 8, 256: 256, 257: 512}
	for in, want := range cases {
		if got := nextPow2(in); got != want {
			t.Errorf("nextPow2(%d) = %d, quería %d", in, got, want)
		}
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := New(256)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("clave-%d", i)
			s.Set(key, []byte("v"))
			if _, ok := s.Get(key); !ok {
				t.Errorf("no encontró %s", key)
			}
		}(i)
	}
	wg.Wait()
}
```

- [ ] **Step 2: Ejecutar y verificar que falla**

Run: `go test ./internal/store/ -v`
Expected: FALLA al compilar con "undefined: New" / "undefined: nextPow2".

- [ ] **Step 3: Implementar `internal/store/store.go`**

```go
// Package store implementa un almacén clave-valor en memoria, dividido en
// shards independientes para reducir la contención entre goroutines.
package store

import (
	"hash/fnv"
	"sync"
)

// shard es una porción del almacén con su propio lock.
type shard struct {
	mu   sync.RWMutex
	data map[string][]byte
}

// Store reparte las claves entre N shards mediante hash(key) & mask.
type Store struct {
	shards []*shard
	mask   uint32
}

// New crea un Store. numShards se redondea hacia arriba a la siguiente
// potencia de 2 (mínimo 1) para poder indexar con una máscara de bits.
func New(numShards int) *Store {
	n := nextPow2(numShards)
	shards := make([]*shard, n)
	for i := range shards {
		shards[i] = &shard{data: make(map[string][]byte)}
	}
	return &Store{shards: shards, mask: uint32(n - 1)}
}

// nextPow2 devuelve la menor potencia de 2 >= n (mínimo 1).
func nextPow2(n int) int {
	if n < 1 {
		return 1
	}
	p := 1
	for p < n {
		p <<= 1
	}
	return p
}

// shardFor selecciona el shard de una clave con FNV-1a de 32 bits.
func (s *Store) shardFor(key string) *shard {
	h := fnv.New32a()
	h.Write([]byte(key))
	return s.shards[h.Sum32()&s.mask]
}

// Set guarda o reemplaza el valor de una clave.
func (s *Store) Set(key string, val []byte) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.data[key] = val
}

// Get devuelve el valor y si la clave existía.
func (s *Store) Get(key string) ([]byte, bool) {
	sh := s.shardFor(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	v, ok := sh.data[key]
	return v, ok
}

// Del borra una clave y devuelve si existía.
func (s *Store) Del(key string) bool {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	if _, ok := sh.data[key]; !ok {
		return false
	}
	delete(sh.data, key)
	return true
}

// Exists indica si la clave existe.
func (s *Store) Exists(key string) bool {
	_, ok := s.Get(key)
	return ok
}
```

- [ ] **Step 4: Ejecutar y verificar que pasa (con -race)**

Run: `go test ./internal/store/ -race -v`
Expected: PASS en los 5 tests, sin avisos de data race.

- [ ] **Step 5: Commit**

```bash
git add internal/store/
git commit -m "feat: almacén clave-valor con sharding y locks por shard"
```

---

### Task 3: Protocolo mini-RESP (`internal/protocol`)

**Files:**
- Create: `internal/protocol/protocol.go`
- Create: `internal/protocol/miniresp.go`
- Test: `internal/protocol/miniresp_test.go`

- [ ] **Step 1: Escribir los tests que fallan**

Crear `internal/protocol/miniresp_test.go`:
```go
package protocol

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestParseValidCommand(t *testing.T) {
	input := "*3\n$3\nSET\n$6\nnombre\n$5\ncesar\n"
	r := bufio.NewReader(strings.NewReader(input))
	cmd, err := MiniRESP{}.Parse(r)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if cmd.Name != "SET" {
		t.Errorf("Name = %q, quería SET", cmd.Name)
	}
	if len(cmd.Args) != 2 || string(cmd.Args[0]) != "nombre" || string(cmd.Args[1]) != "cesar" {
		t.Errorf("Args = %q", cmd.Args)
	}
}

func TestParseValueWithSpacesAndNewline(t *testing.T) {
	val := "hola mundo\ncon salto"
	input := fmt.Sprintf("*3\n$3\nSET\n$1\nk\n$%d\n%s\n", len(val), val)
	r := bufio.NewReader(strings.NewReader(input))
	cmd, err := MiniRESP{}.Parse(r)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if string(cmd.Args[1]) != val {
		t.Errorf("valor = %q, quería %q", cmd.Args[1], val)
	}
}

func TestParseMalformed(t *testing.T) {
	for _, input := range []string{"SET k v\n", "*abc\n", "*2\nfoo\n"} {
		r := bufio.NewReader(strings.NewReader(input))
		if _, err := (MiniRESP{}).Parse(r); err == nil {
			t.Errorf("esperaba error para %q", input)
		}
	}
}

func TestEncodeReplies(t *testing.T) {
	cases := []struct {
		reply Reply
		want  string
	}{
		{StatusReply{Msg: "OK"}, "+OK\n"},
		{ErrorReply{Msg: "ERR x"}, "-ERR x\n"},
		{IntReply{N: 3}, ":3\n"},
		{BulkReply{Value: []byte("hi")}, "$2\nhi\n"},
		{BulkReply{Null: true}, "$-1\n"},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		if err := (MiniRESP{}).Encode(&buf, c.reply); err != nil {
			t.Fatalf("Encode error: %v", err)
		}
		if buf.String() != c.want {
			t.Errorf("Encode(%#v) = %q, quería %q", c.reply, buf.String(), c.want)
		}
	}
}
```

- [ ] **Step 2: Ejecutar y verificar que falla**

Run: `go test ./internal/protocol/ -v`
Expected: FALLA al compilar con "undefined: MiniRESP" y los tipos `Reply`.

- [ ] **Step 3: Crear `internal/protocol/protocol.go` (tipos e interfaz)**

```go
// Package protocol define cómo se leen comandos y se escriben respuestas por
// el socket. La interfaz Protocol permite cambiar el formato de cable
// (mini-RESP hoy, RESP real en el futuro) sin tocar el resto del servidor.
package protocol

import (
	"bufio"
	"io"
)

// Command es una orden ya parseada: nombre en mayúsculas + argumentos en bytes.
type Command struct {
	Name string
	Args [][]byte
}

// Reply es cualquier respuesta que el protocolo sabe serializar.
type Reply interface {
	isReply()
}

// StatusReply es una respuesta de estado simple (p.ej. OK).
type StatusReply struct{ Msg string }

// ErrorReply es un error legible para el cliente.
type ErrorReply struct{ Msg string }

// IntReply es una respuesta entera.
type IntReply struct{ N int64 }

// BulkReply es un valor binario; Null indica ausencia de valor.
type BulkReply struct {
	Value []byte
	Null  bool
}

func (StatusReply) isReply() {}
func (ErrorReply) isReply()  {}
func (IntReply) isReply()    {}
func (BulkReply) isReply()   {}

// Protocol abstrae la lectura de comandos y la escritura de respuestas.
type Protocol interface {
	Parse(r *bufio.Reader) (Command, error)
	Encode(w io.Writer, reply Reply) error
}
```

- [ ] **Step 4: Crear `internal/protocol/miniresp.go` (implementación)**

```go
package protocol

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// ErrProtocol indica una petición que no respeta el formato mini-RESP.
var ErrProtocol = errors.New("protocolo malformado")

// MiniRESP implementa Protocol con marco propio terminado en '\n':
//
//	petición:  *N\n  luego N veces  $len\n<bytes>\n
//	respuesta: +status\n | -err\n | :int\n | $len\n<bytes>\n | $-1\n
type MiniRESP struct{}

// Parse lee una orden completa del reader.
func (MiniRESP) Parse(r *bufio.Reader) (Command, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return Command{}, err
	}
	line = strings.TrimRight(line, "\r\n")
	if len(line) == 0 || line[0] != '*' {
		return Command{}, ErrProtocol
	}
	n, err := strconv.Atoi(line[1:])
	if err != nil || n < 1 {
		return Command{}, ErrProtocol
	}
	parts := make([][]byte, n)
	for i := 0; i < n; i++ {
		b, err := readBulk(r)
		if err != nil {
			return Command{}, err
		}
		parts[i] = b
	}
	return Command{Name: strings.ToUpper(string(parts[0])), Args: parts[1:]}, nil
}

// readBulk lee una parte con prefijo de longitud: $len\n<bytes>\n.
func readBulk(r *bufio.Reader) ([]byte, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimRight(line, "\r\n")
	if len(line) == 0 || line[0] != '$' {
		return nil, ErrProtocol
	}
	n, err := strconv.Atoi(line[1:])
	if err != nil || n < 0 {
		return nil, ErrProtocol
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	// consumir el '\n' (y posible '\r') que cierra los bytes
	if _, err := r.ReadString('\n'); err != nil {
		return nil, err
	}
	return buf, nil
}

// Encode serializa una respuesta al writer.
func (MiniRESP) Encode(w io.Writer, reply Reply) error {
	switch r := reply.(type) {
	case StatusReply:
		_, err := fmt.Fprintf(w, "+%s\n", r.Msg)
		return err
	case ErrorReply:
		_, err := fmt.Fprintf(w, "-%s\n", r.Msg)
		return err
	case IntReply:
		_, err := fmt.Fprintf(w, ":%d\n", r.N)
		return err
	case BulkReply:
		if r.Null {
			_, err := io.WriteString(w, "$-1\n")
			return err
		}
		if _, err := fmt.Fprintf(w, "$%d\n", len(r.Value)); err != nil {
			return err
		}
		if _, err := w.Write(r.Value); err != nil {
			return err
		}
		_, err := io.WriteString(w, "\n")
		return err
	default:
		return fmt.Errorf("tipo de respuesta desconocido: %T", reply)
	}
}
```

- [ ] **Step 5: Ejecutar y verificar que pasa**

Run: `go test ./internal/protocol/ -v`
Expected: PASS en los 4 tests.

- [ ] **Step 6: Commit**

```bash
git add internal/protocol/
git commit -m "feat: protocolo mini-RESP con prefijo de longitud (parse/encode)"
```

---

### Task 4: Dispatcher y comandos (`internal/command`)

**Files:**
- Create: `internal/command/command.go`
- Create: `internal/command/handlers.go`
- Test: `internal/command/command_test.go`

- [ ] **Step 1: Escribir los tests que fallan**

Crear `internal/command/command_test.go`:
```go
package command

import (
	"testing"

	"llavero/internal/protocol"
	"llavero/internal/store"
)

// dispatch es un helper que convierte argumentos string a [][]byte y despacha.
func dispatch(d *Dispatcher, s *store.Store, name string, args ...string) protocol.Reply {
	bargs := make([][]byte, len(args))
	for i, a := range args {
		bargs[i] = []byte(a)
	}
	return d.Dispatch(s, protocol.Command{Name: name, Args: bargs})
}

func TestSetGetDelExists(t *testing.T) {
	d := NewDispatcher()
	s := store.New(16)

	if r := dispatch(d, s, "SET", "k", "v"); r != (protocol.StatusReply{Msg: "OK"}) {
		t.Fatalf("SET devolvió %#v", r)
	}
	r := dispatch(d, s, "GET", "k")
	if b, ok := r.(protocol.BulkReply); !ok || string(b.Value) != "v" {
		t.Fatalf("GET devolvió %#v", r)
	}
	if r := dispatch(d, s, "EXISTS", "k"); r != (protocol.IntReply{N: 1}) {
		t.Fatalf("EXISTS devolvió %#v", r)
	}
	if r := dispatch(d, s, "DEL", "k"); r != (protocol.IntReply{N: 1}) {
		t.Fatalf("DEL devolvió %#v", r)
	}
	if r := dispatch(d, s, "GET", "k"); func() bool { b, ok := r.(protocol.BulkReply); return !ok || !b.Null }() {
		t.Fatalf("GET tras DEL debería ser bulk nulo, fue %#v", r)
	}
}

func TestPingReply(t *testing.T) {
	d := NewDispatcher()
	s := store.New(16)
	if r := dispatch(d, s, "PING"); r != (protocol.StatusReply{Msg: "PONG"}) {
		t.Fatalf("PING devolvió %#v", r)
	}
}

func TestUnknownAndArity(t *testing.T) {
	d := NewDispatcher()
	s := store.New(16)
	if _, ok := dispatch(d, s, "NOPE").(protocol.ErrorReply); !ok {
		t.Errorf("comando desconocido debería dar ErrorReply")
	}
	if _, ok := dispatch(d, s, "GET").(protocol.ErrorReply); !ok {
		t.Errorf("GET sin args debería dar ErrorReply")
	}
	if _, ok := dispatch(d, s, "SET", "solo-clave").(protocol.ErrorReply); !ok {
		t.Errorf("SET con 1 arg debería dar ErrorReply")
	}
}
```

- [ ] **Step 2: Ejecutar y verificar que falla**

Run: `go test ./internal/command/ -v`
Expected: FALLA al compilar con "undefined: NewDispatcher".

- [ ] **Step 3: Crear `internal/command/command.go` (dispatcher)**

```go
// Package command mapea nombres de comando a handlers que operan sobre el store
// y devuelven una respuesta del protocolo.
package command

import (
	"strings"

	"llavero/internal/protocol"
	"llavero/internal/store"
)

// Handler ejecuta un comando sobre el store y devuelve una respuesta.
type Handler func(s *store.Store, args [][]byte) protocol.Reply

// Dispatcher resuelve el handler de cada comando por nombre.
type Dispatcher struct {
	handlers map[string]Handler
}

// NewDispatcher crea un dispatcher con los comandos soportados registrados.
func NewDispatcher() *Dispatcher {
	d := &Dispatcher{handlers: make(map[string]Handler)}
	d.handlers["PING"] = cmdPing
	d.handlers["GET"] = cmdGet
	d.handlers["SET"] = cmdSet
	d.handlers["DEL"] = cmdDel
	d.handlers["EXISTS"] = cmdExists
	return d
}

// Dispatch ejecuta el comando o devuelve un error si no existe.
func (d *Dispatcher) Dispatch(s *store.Store, cmd protocol.Command) protocol.Reply {
	h, ok := d.handlers[strings.ToUpper(cmd.Name)]
	if !ok {
		return protocol.ErrorReply{Msg: "ERR comando desconocido: " + cmd.Name}
	}
	return h(s, cmd.Args)
}
```

- [ ] **Step 4: Crear `internal/command/handlers.go` (handlers)**

```go
package command

import (
	"llavero/internal/protocol"
	"llavero/internal/store"
)

func cmdPing(_ *store.Store, args [][]byte) protocol.Reply {
	if len(args) > 0 {
		return protocol.BulkReply{Value: args[0]}
	}
	return protocol.StatusReply{Msg: "PONG"}
}

func cmdGet(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR GET requiere 1 argumento"}
	}
	v, ok := s.Get(string(args[0]))
	if !ok {
		return protocol.BulkReply{Null: true}
	}
	return protocol.BulkReply{Value: v}
}

func cmdSet(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 2 {
		return protocol.ErrorReply{Msg: "ERR SET requiere 2 argumentos"}
	}
	s.Set(string(args[0]), args[1])
	return protocol.StatusReply{Msg: "OK"}
}

func cmdDel(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) < 1 {
		return protocol.ErrorReply{Msg: "ERR DEL requiere al menos 1 argumento"}
	}
	var n int64
	for _, a := range args {
		if s.Del(string(a)) {
			n++
		}
	}
	return protocol.IntReply{N: n}
}

func cmdExists(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR EXISTS requiere 1 argumento"}
	}
	if s.Exists(string(args[0])) {
		return protocol.IntReply{N: 1}
	}
	return protocol.IntReply{N: 0}
}
```

- [ ] **Step 5: Ejecutar y verificar que pasa**

Run: `go test ./internal/command/ -v`
Expected: PASS en los 3 tests.

- [ ] **Step 6: Commit**

```bash
git add internal/command/
git commit -m "feat: dispatcher y comandos GET/SET/DEL/EXISTS/PING"
```

---

### Task 5: Reconectar el servidor al núcleo KV (`internal/server`)

**Files:**
- Modify: `internal/server/server.go` (reescritura completa)
- Modify: `internal/server/server_test.go` (reescritura completa)

- [ ] **Step 1: Reescribir `internal/server/server_test.go` a mini-RESP**

Reemplazar TODO el contenido de `internal/server/server_test.go` por:
```go
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
```

- [ ] **Step 2: Ejecutar y verificar que falla**

Run: `go test ./internal/server/ -v`
Expected: FALLA — el servidor aún usa el protocolo de líneas viejo, así que
`TestPingReturnsPong` recibe algo distinto de `+PONG` (o la conexión se cuelga
esperando). Confirma que el test rojo es por el cambio de protocolo.

- [ ] **Step 3: Reescribir `internal/server/server.go`**

Reemplazar TODO el contenido de `internal/server/server.go` por:
```go
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
```

- [ ] **Step 4: Ejecutar y verificar que pasa (con -race)**

Run: `go test ./internal/server/ -race -v`
Expected: PASS en los 5 tests, sin avisos de data race.

- [ ] **Step 5: Commit**

```bash
git add internal/server/server.go internal/server/server_test.go
git commit -m "feat: servidor usa mini-RESP + dispatcher + store"
```

---

### Task 6: Verificación final de la fase

**Files:** ninguno (solo verificación).

- [ ] **Step 1: vet + build + suite completa con -race**

Run: `go vet ./... && go build ./... && go test ./... -race`
Expected: sin avisos de vet, build OK, todos los paquetes en verde.

- [ ] **Step 2: Prueba de humo manual con un cliente mini-RESP**

Crear `/tmp/smoke2.go`:
```go
package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"time"
)

func send(w *bufio.Writer, parts ...string) {
	fmt.Fprintf(w, "*%d\n", len(parts))
	for _, p := range parts {
		fmt.Fprintf(w, "$%d\n%s\n", len(p), p)
	}
	w.Flush()
}

func main() {
	var conn net.Conn
	var err error
	for i := 0; i < 30; i++ {
		if conn, err = net.Dial("tcp", "localhost:6380"); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		fmt.Println("no conecta:", err)
		os.Exit(1)
	}
	defer conn.Close()
	w := bufio.NewWriter(conn)
	r := bufio.NewReader(conn)

	send(w, "SET", "saludo", "hola mundo")
	line, _ := r.ReadString('\n')
	fmt.Printf("SET    -> %q\n", line)

	send(w, "GET", "saludo")
	hdr, _ := r.ReadString('\n')
	body, _ := r.ReadString('\n')
	fmt.Printf("GET    -> %q %q\n", hdr, body)

	send(w, "EXISTS", "saludo")
	line, _ = r.ReadString('\n')
	fmt.Printf("EXISTS -> %q\n", line)

	send(w, "DEL", "saludo")
	line, _ = r.ReadString('\n')
	fmt.Printf("DEL    -> %q\n", line)

	send(w, "GET", "saludo")
	line, _ = r.ReadString('\n')
	fmt.Printf("GET    -> %q (nulo)\n", line)
}
```

Run:
```bash
go run ./cmd/llavero &
SRV=$!
go run /tmp/smoke2.go
kill $SRV 2>/dev/null
pkill -f 'exe/llavero' 2>/dev/null
rm -f /tmp/smoke2.go
```
Expected:
```
SET    -> "+OK\n"
GET    -> "$10\n" "hola mundo\n"
EXISTS -> ":1\n"
DEL    -> ":1\n"
GET    -> "$-1\n" (nulo)
```

- [ ] **Step 3: (Opcional) recarga en caliente con Air**

Si Air está instalado (`go install github.com/air-verse/air@latest`), ejecutar
`air` en la raíz y comprobar que al guardar un `.go` recompila y reinicia el
servidor automáticamente. Detener con Ctrl-C.

---

## Resultado de la fase

Al terminar la Fase 2, Llavero será un almacén clave-valor real: protocolo
mini-RESP con prefijo de longitud, almacén con 256 shards y locks por shard,
comandos `GET`/`SET`/`DEL`/`EXISTS`/`PING`, y recarga en caliente con Air para
desarrollo. La Fase 3 (TTL/expiración) refactorizará el valor del shard de
`[]byte` a una entrada con instante de expiración.
