# Llavero Fase 3 — Plan de Implementación (TTL / Expiración)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Dar a Llavero expiración de claves: comandos `EXPIRE`/`TTL`/`PERSIST`, borrado perezoso al acceder, y una expiración activa adaptativa (estilo Redis) en segundo plano con apagado limpio.

**Architecture:** `internal/store` cambia su valor interno de `[]byte` a un `entry{value, expireAt}` y gana métodos de TTL más una `ActiveExpireCycle()` adaptativa (muestrea claves con TTL, borra vencidas, repite mientras la proporción de vencidas sea alta). `internal/command` añade tres handlers. `internal/server` lanza una goroutine con un `time.Ticker` que ejecuta la expiración activa y la detiene al cerrar (canal `stop` + `sync.Once`).

**Tech Stack:** Go 1.26, librería estándar (`time`, `sync`, `strconv`, `hash/fnv`). Sin dependencias externas.

## Semántica (estilo Redis)

- `EXPIRE key segundos` → `:1` si la clave existía y se aplicó; `:0` si no existe.
- `TTL key` → `:segundos` restantes; `:-1` si existe sin expiración; `:-2` si no existe.
- `PERSIST key` → `:1` si quitó una expiración; `:0` si no había o no existe.
- `SET` limpia cualquier TTL previo de la clave.
- Borrado perezoso: al leer una clave vencida (`Get`/`Exists`/`TTL`), se borra y se trata como inexistente.

## Estructura de archivos

- `internal/store/store.go` — reescritura: modelo `entry` + métodos TTL + expiración activa.
- `internal/store/store_test.go` — se AÑADEN tests (los existentes siguen válidos).
- `internal/command/command.go` — registrar `EXPIRE`/`TTL`/`PERSIST`.
- `internal/command/handlers.go` — añadir los tres handlers.
- `internal/command/command_test.go` — añadir test de los comandos.
- `internal/server/server.go` — reescritura: goroutine de expiración + apagado limpio.
- `internal/server/server_test.go` — añadir test de apagado idempotente.

---

### Task 1: Modelo con TTL y expiración activa (`internal/store`)

**Files:**
- Modify (full rewrite): `internal/store/store.go`
- Modify (add tests): `internal/store/store_test.go`

- [ ] **Step 1: Añadir los tests que fallan**

Primero, en `internal/store/store_test.go`, cambiar el bloque de imports para incluir `"time"`. El import actual es:
```go
import (
	"fmt"
	"sync"
	"testing"
)
```
Reemplazarlo por:
```go
import (
	"fmt"
	"sync"
	"testing"
	"time"
)
```

Luego AÑADIR al final de `internal/store/store_test.go` estos tests:
```go
func TestSetClearsTTL(t *testing.T) {
	s := New(16)
	s.Set("k", []byte("v"))
	s.Expire("k", 100*time.Second)
	s.Set("k", []byte("v2")) // debe limpiar el TTL
	_, exists, hasExpiry := s.TTL("k")
	if !exists || hasExpiry {
		t.Fatalf("tras Set, exists=%v hasExpiry=%v; quería exists=true hasExpiry=false", exists, hasExpiry)
	}
}

func TestExpireAndTTL(t *testing.T) {
	s := New(16)
	s.Set("k", []byte("v"))
	if !s.Expire("k", 100*time.Second) {
		t.Fatal("Expire de clave existente devolvió false")
	}
	rem, exists, hasExpiry := s.TTL("k")
	if !exists || !hasExpiry {
		t.Fatalf("exists=%v hasExpiry=%v", exists, hasExpiry)
	}
	if rem <= 0 || rem > 100*time.Second {
		t.Fatalf("restante fuera de rango: %v", rem)
	}
	if s.Expire("nope", time.Second) {
		t.Fatal("Expire de clave inexistente devolvió true")
	}
}

func TestLazyExpireOnGet(t *testing.T) {
	s := New(16)
	s.Set("k", []byte("v"))
	s.Expire("k", -time.Second) // ya vencida
	if _, ok := s.Get("k"); ok {
		t.Fatal("Get devolvió una clave vencida")
	}
}

func TestPersist(t *testing.T) {
	s := New(16)
	s.Set("k", []byte("v"))
	s.Expire("k", 100*time.Second)
	if !s.Persist("k") {
		t.Fatal("Persist devolvió false con TTL presente")
	}
	if _, _, hasExpiry := s.TTL("k"); hasExpiry {
		t.Fatal("seguía con expiración tras Persist")
	}
	if s.Persist("k") {
		t.Fatal("Persist devolvió true sin TTL que quitar")
	}
}

func TestActiveExpireCycleRemovesExpired(t *testing.T) {
	s := New(1) // un solo shard para un test determinista
	for i := 0; i < 50; i++ {
		k := fmt.Sprintf("exp-%d", i)
		s.Set(k, []byte("v"))
		s.Expire(k, -time.Second) // todas vencidas
	}
	s.Set("vivo", []byte("v"))
	s.Set("futuro", []byte("v"))
	s.Expire("futuro", time.Hour)

	s.ActiveExpireCycle()

	total := 0
	for _, sh := range s.shards {
		total += len(sh.data)
	}
	if total != 2 {
		t.Fatalf("tras la ronda quedaban %d claves, quería 2 (vivo y futuro)", total)
	}
	if _, ok := s.Get("vivo"); !ok {
		t.Error("se borró una clave sin TTL")
	}
	if _, ok := s.Get("futuro"); !ok {
		t.Error("se borró una clave con TTL futuro")
	}
}
```

- [ ] **Step 2: Ejecutar y verificar que falla**

Run: `go test ./internal/store/ -v`
Expected: FALLA al compilar con "s.Expire undefined", "s.TTL undefined", "s.Persist undefined", "s.ActiveExpireCycle undefined".

- [ ] **Step 3: Reescribir `internal/store/store.go`**

Reemplazar TODO el contenido de `internal/store/store.go` por:
```go
// Package store implementa un almacén clave-valor en memoria, dividido en
// shards independientes para reducir la contención entre goroutines. Cada
// entrada puede tener una expiración (TTL) opcional.
package store

import (
	"hash/fnv"
	"sync"
	"time"
)

// entry es el valor almacenado: bytes + una expiración opcional.
// Un expireAt cero significa "sin expiración".
type entry struct {
	value    []byte
	expireAt time.Time
}

// expired indica si la entrada ya venció respecto a now.
func (e *entry) expired(now time.Time) bool {
	return !e.expireAt.IsZero() && now.After(e.expireAt)
}

// shard es una porción del almacén con su propio lock.
type shard struct {
	mu   sync.RWMutex
	data map[string]*entry
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
		shards[i] = &shard{data: make(map[string]*entry)}
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

// Set guarda o reemplaza el valor de una clave, limpiando cualquier TTL previo.
func (s *Store) Set(key string, val []byte) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.data[key] = &entry{value: val}
}

// Get devuelve el valor y si la clave existe. Si está vencida, la borra
// (expiración perezosa) y la trata como inexistente.
func (s *Store) Get(key string) ([]byte, bool) {
	sh := s.shardFor(key)
	now := time.Now()

	sh.mu.RLock()
	e, ok := sh.data[key]
	if ok && !e.expired(now) {
		v := e.value
		sh.mu.RUnlock()
		return v, true
	}
	sh.mu.RUnlock()

	if ok {
		// estaba vencida: borrarla con lock de escritura, re-comprobando
		sh.mu.Lock()
		if e2, ok2 := sh.data[key]; ok2 && e2.expired(time.Now()) {
			delete(sh.data, key)
		}
		sh.mu.Unlock()
	}
	return nil, false
}

// Exists indica si la clave existe (aplicando expiración perezosa).
func (s *Store) Exists(key string) bool {
	_, ok := s.Get(key)
	return ok
}

// Del borra una clave y devuelve si existía de forma efectiva (una clave
// vencida cuenta como inexistente, aunque se elimine del mapa).
func (s *Store) Del(key string) bool {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.data[key]
	if !ok {
		return false
	}
	delete(sh.data, key)
	return !e.expired(time.Now())
}

// Expire fija una expiración relativa para una clave existente. Devuelve si la
// clave existía y no estaba vencida.
func (s *Store) Expire(key string, d time.Duration) bool {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.data[key]
	if !ok || e.expired(time.Now()) {
		if ok {
			delete(sh.data, key)
		}
		return false
	}
	e.expireAt = time.Now().Add(d)
	return true
}

// Persist quita la expiración de una clave. Devuelve si había un TTL que quitar.
func (s *Store) Persist(key string) bool {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.data[key]
	if !ok || e.expired(time.Now()) {
		if ok {
			delete(sh.data, key)
		}
		return false
	}
	if e.expireAt.IsZero() {
		return false
	}
	e.expireAt = time.Time{}
	return true
}

// TTL devuelve el tiempo restante de una clave, si existe y si tiene expiración.
func (s *Store) TTL(key string) (remaining time.Duration, exists bool, hasExpiry bool) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.data[key]
	if !ok {
		return 0, false, false
	}
	now := time.Now()
	if e.expired(now) {
		delete(sh.data, key)
		return 0, false, false
	}
	if e.expireAt.IsZero() {
		return 0, true, false
	}
	return e.expireAt.Sub(now), true, true
}

// expireSampleSize es cuántas claves con TTL se muestrean por ronda y shard.
const expireSampleSize = 20

// ActiveExpireCycle ejecuta una ronda de expiración activa adaptativa sobre
// todos los shards. Está pensada para llamarse periódicamente.
func (s *Store) ActiveExpireCycle() {
	for _, sh := range s.shards {
		expireShard(sh)
	}
}

// expireShard muestrea claves con TTL del shard y borra las vencidas; repite
// mientras una proporción alta (>=25%) de la muestra siga venciendo, con un
// tope de rondas para acotar el tiempo. El orden aleatorio de iteración de los
// mapas de Go nos da el muestreo.
func expireShard(sh *shard) {
	const maxRounds = 16
	now := time.Now()
	for round := 0; round < maxRounds; round++ {
		sh.mu.Lock()
		sampled, expired := 0, 0
		for key, e := range sh.data {
			if e.expireAt.IsZero() {
				continue // solo cuentan claves con TTL
			}
			sampled++
			if e.expired(now) {
				delete(sh.data, key)
				expired++
			}
			if sampled >= expireSampleSize {
				break
			}
		}
		sh.mu.Unlock()
		// si no había claves con TTL o pocas estaban vencidas (<25%), parar
		if sampled == 0 || expired*4 < sampled {
			return
		}
	}
}
```

- [ ] **Step 4: Ejecutar y verificar que pasa (con -race)**

Run: `go test ./internal/store/ -race -v`
Expected: PASS en todos los tests (los previos: TestSetThenGet, TestGetMissing, TestDelAndExists, TestNextPow2, TestConcurrentAccess; y los nuevos), sin data races.

- [ ] **Step 5: Commit**

```bash
git add internal/store/
git -c user.email="developert@orvixapp.com" -c user.name="cesar" commit -m "feat: TTL en el store (entry con expiración + lazy + expiración activa)"
```

---

### Task 2: Comandos EXPIRE / TTL / PERSIST (`internal/command`)

**Files:**
- Modify: `internal/command/command.go` (registrar handlers)
- Modify: `internal/command/handlers.go` (añadir handlers)
- Modify: `internal/command/command_test.go` (añadir test)

- [ ] **Step 1: Añadir el test que falla**

AÑADIR al final de `internal/command/command_test.go`:
```go
func TestExpireTtlPersist(t *testing.T) {
	d := NewDispatcher()
	s := store.New(16)
	dispatch(d, s, "SET", "k", "v")

	// TTL de clave sin expiración → -1
	if r := dispatch(d, s, "TTL", "k"); r != (protocol.IntReply{N: -1}) {
		t.Fatalf("TTL sin expiración → %#v", r)
	}
	// EXPIRE existente → 1
	if r := dispatch(d, s, "EXPIRE", "k", "100"); r != (protocol.IntReply{N: 1}) {
		t.Fatalf("EXPIRE → %#v", r)
	}
	// TTL ahora ~100 (margen por el redondeo)
	r := dispatch(d, s, "TTL", "k")
	ir, ok := r.(protocol.IntReply)
	if !ok || ir.N < 95 || ir.N > 100 {
		t.Fatalf("TTL tras EXPIRE → %#v", r)
	}
	// PERSIST quita el TTL → 1
	if r := dispatch(d, s, "PERSIST", "k"); r != (protocol.IntReply{N: 1}) {
		t.Fatalf("PERSIST → %#v", r)
	}
	if r := dispatch(d, s, "TTL", "k"); r != (protocol.IntReply{N: -1}) {
		t.Fatalf("TTL tras PERSIST → %#v", r)
	}
	// TTL de clave inexistente → -2
	if r := dispatch(d, s, "TTL", "nope"); r != (protocol.IntReply{N: -2}) {
		t.Fatalf("TTL inexistente → %#v", r)
	}
	// EXPIRE de clave inexistente → 0
	if r := dispatch(d, s, "EXPIRE", "nope", "10"); r != (protocol.IntReply{N: 0}) {
		t.Fatalf("EXPIRE inexistente → %#v", r)
	}
	// EXPIRE con segundos no numéricos → error
	if _, ok := dispatch(d, s, "EXPIRE", "k", "abc").(protocol.ErrorReply); !ok {
		t.Errorf("EXPIRE con segundos no numéricos debería dar ErrorReply")
	}
}
```

- [ ] **Step 2: Ejecutar y verificar que falla**

Run: `go test ./internal/command/ -run TestExpireTtlPersist -v`
Expected: FALLA — el dispatcher devuelve `ErrorReply` "comando desconocido" para EXPIRE/TTL/PERSIST, así que las comparaciones con `IntReply` fallan.

- [ ] **Step 3: Registrar los handlers en `internal/command/command.go`**

En `NewDispatcher`, tras la línea `d.handlers["EXISTS"] = cmdExists`, añadir:
```go
	d.handlers["EXPIRE"] = cmdExpire
	d.handlers["TTL"] = cmdTTL
	d.handlers["PERSIST"] = cmdPersist
```

- [ ] **Step 4: Añadir los handlers en `internal/command/handlers.go`**

Cambiar el bloque de imports de `internal/command/handlers.go`. El actual es:
```go
import (
	"llavero/internal/protocol"
	"llavero/internal/store"
)
```
Reemplazarlo por:
```go
import (
	"strconv"
	"time"

	"llavero/internal/protocol"
	"llavero/internal/store"
)
```
Y AÑADIR al final del archivo:
```go
func cmdExpire(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 2 {
		return protocol.ErrorReply{Msg: "ERR EXPIRE requiere 2 argumentos"}
	}
	secs, err := strconv.Atoi(string(args[1]))
	if err != nil {
		return protocol.ErrorReply{Msg: "ERR el TTL debe ser un entero"}
	}
	if s.Expire(string(args[0]), time.Duration(secs)*time.Second) {
		return protocol.IntReply{N: 1}
	}
	return protocol.IntReply{N: 0}
}

func cmdTTL(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR TTL requiere 1 argumento"}
	}
	rem, exists, hasExpiry := s.TTL(string(args[0]))
	switch {
	case !exists:
		return protocol.IntReply{N: -2}
	case !hasExpiry:
		return protocol.IntReply{N: -1}
	default:
		// redondeo hacia arriba a segundos
		secs := int64((rem + time.Second - 1) / time.Second)
		return protocol.IntReply{N: secs}
	}
}

func cmdPersist(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR PERSIST requiere 1 argumento"}
	}
	if s.Persist(string(args[0])) {
		return protocol.IntReply{N: 1}
	}
	return protocol.IntReply{N: 0}
}
```

- [ ] **Step 5: Ejecutar y verificar que pasa**

Run: `go test ./internal/command/ -v`
Expected: PASS en todos los tests (los previos y el nuevo TestExpireTtlPersist).

- [ ] **Step 6: Commit**

```bash
git add internal/command/
git -c user.email="developert@orvixapp.com" -c user.name="cesar" commit -m "feat: comandos EXPIRE/TTL/PERSIST"
```

---

### Task 3: Expiración activa en el servidor + apagado limpio (`internal/server`)

**Files:**
- Modify (full rewrite): `internal/server/server.go`
- Modify (add test): `internal/server/server_test.go`

- [ ] **Step 1: Añadir el test que falla**

AÑADIR al final de `internal/server/server_test.go`:
```go
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
```

- [ ] **Step 2: Ejecutar y verificar que falla**

Run: `go test ./internal/server/ -run TestCloseIsIdempotent -v`
Expected: FALLA — con el `server.go` actual (Fase 2), el segundo `Close()` cierra de nuevo el listener pero NO hay canal `stop` ni `sync.Once`; el test compila pero, sobre todo, tras añadir la expiración activa el doble cierre del canal entraría en pánico sin la protección. (Si con el server de Fase 2 el test pasa trivialmente, igualmente continúa: el Step 3 introduce la goroutine que hace necesaria la protección.)

- [ ] **Step 3: Reescribir `internal/server/server.go`**

Reemplazar TODO el contenido de `internal/server/server.go` por:
```go
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
```

- [ ] **Step 4: Ejecutar y verificar que pasa (con -race)**

Run: `go test ./internal/server/ -race -v`
Expected: PASS en todos los tests (los de Fase 2 + TestCloseIsIdempotent), sin data races ni pánicos por doble cierre.

- [ ] **Step 5: Commit**

```bash
git add internal/server/
git -c user.email="developert@orvixapp.com" -c user.name="cesar" commit -m "feat: expiración activa periódica en el servidor con apagado limpio"
```

---

### Task 4: Verificación final de la fase

**Files:** ninguno (solo verificación).

- [ ] **Step 1: vet + build + suite completa con -race**

Run: `go vet ./... && go build ./... && go test ./... -race`
Expected: vet sin avisos, build OK, todos los paquetes en verde.

- [ ] **Step 2: Prueba de humo manual de TTL**

Crear `/tmp/smoke3.go`:
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
	line := func(label string) {
		l, _ := r.ReadString('\n')
		fmt.Printf("%-22s -> %q\n", label, l)
	}

	send(w, "SET", "sesion", "abc")
	line("SET sesion abc")
	send(w, "TTL", "sesion")
	line("TTL (sin expirar)")
	send(w, "EXPIRE", "sesion", "100")
	line("EXPIRE 100")
	send(w, "TTL", "sesion")
	line("TTL (tras expire)")
	send(w, "PERSIST", "sesion")
	line("PERSIST")
	send(w, "TTL", "sesion")
	line("TTL (tras persist)")
	send(w, "EXPIRE", "sesion", "1")
	line("EXPIRE 1")
	fmt.Println("... esperando 1.3s a que venza ...")
	time.Sleep(1300 * time.Millisecond)
	send(w, "GET", "sesion")
	line("GET (debería ser nulo)")
}
```

Run:
```bash
go build -o /tmp/llavero-bin ./cmd/llavero
/tmp/llavero-bin &
SRV=$!
go run /tmp/smoke3.go
kill $SRV 2>/dev/null
rm -f /tmp/smoke3.go /tmp/llavero-bin
```
Expected (aproximado):
```
SET sesion abc         -> "+OK\n"
TTL (sin expirar)      -> ":-1\n"
EXPIRE 100             -> ":1\n"
TTL (tras expire)      -> ":100\n"
PERSIST                -> ":1\n"
TTL (tras persist)     -> ":-1\n"
EXPIRE 1               -> ":1\n"
... esperando 1.3s a que venza ...
GET (debería ser nulo) -> "$-1\n"
```

Nota de operación: si un servidor anterior quedó ocupando el puerto 6380, matarlo
por PID (`ss -ltnp | grep 6380`) antes de arrancar el nuevo binario.

---

## Resultado de la fase

Al terminar la Fase 3, Llavero soporta expiración de claves: `EXPIRE`/`TTL`/`PERSIST`,
borrado perezoso al acceder y una expiración activa adaptativa en segundo plano que
se apaga limpiamente con el servidor. La Fase 4 (persistencia: snapshot + carga)
serializará el estado de los shards a disco.
