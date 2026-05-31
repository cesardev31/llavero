# Llavero Fase 9 — Plan de Implementación (Pub/Sub)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Añadir mensajería publish/subscribe: `SUBSCRIBE`/`UNSUBSCRIBE`/`PUBLISH`, con mensajes empujados a las conexiones suscritas.

**Architecture:** Un `internal/pubsub.Broker` enruta `canal → suscriptores`. Cada conexión se modela como un `client` con escritura serializada (`writeMu` + `send`) que implementa la interfaz `pubsub.Subscriber`. Los comandos pub/sub se manejan en el servidor (son estado de conexión); el resto sigue yendo al dispatcher. `handleConn` se reestructura para crear el `client`, enrutar comandos y dar de baja las suscripciones al desconectar.

**Tech Stack:** Go 1.26, librería estándar (`sync`, `net`, `strings`, `bufio`, `io`, `strconv`). Sin dependencias externas.

## Semántica (estilo Redis)

- `SUBSCRIBE c1 [c2...]`: por cada canal responde un array `["subscribe", canal, nº_suscripciones]`.
- `UNSUBSCRIBE [c1...]`: por cada canal responde `["unsubscribe", canal, nº_restantes]`. Sin argumentos = darse de baja de todos; si no había ninguna, responde `["unsubscribe", (nil), 0]`.
- `PUBLISH canal mensaje` → `:nº_receptores`.
- Una conexión suscrita recibe arrays `["message", canal, payload]` cuando alguien publica.
- Decisiones de simplificación: entrega síncrona bajo el mutex de escritura; no se fuerza el "modo suscripción" (una conexión suscrita puede seguir ejecutando comandos normales); solo canales exactos (sin `PSUBSCRIBE`).

## Estructura de archivos

- `internal/pubsub/broker.go` + `broker_test.go` — el broker y la interfaz `Subscriber`.
- `internal/server/pubsub.go` — `client` (send/Deliver) y handlers `cmdSubscribe`/`cmdUnsubscribe`/`cmdPublish`.
- `internal/server/server.go` — campo `broker`, init, `handleConn`/`handleCommand`/`unsubscribeAll`.
- `internal/server/server_test.go` — test pub/sub sobre TCP + helpers de lectura RESP.

---

### Task 1: Broker pub/sub (`internal/pubsub`)

**Files:**
- Create: `internal/pubsub/broker.go`
- Create: `internal/pubsub/broker_test.go`

- [ ] **Step 1: Escribir los tests que fallan** — crear `internal/pubsub/broker_test.go`:
```go
package pubsub

import (
	"sync"
	"testing"
)

// recorder es un Subscriber de prueba que registra lo que recibe.
type recorder struct {
	mu  sync.Mutex
	got []string
}

func (r *recorder) Deliver(channel string, payload []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got = append(r.got, channel+":"+string(payload))
}

func (r *recorder) list() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.got))
	copy(out, r.got)
	return out
}

func TestPublishDeliversToSubscribers(t *testing.T) {
	b := New()
	a := &recorder{}
	c := &recorder{}
	b.Subscribe(a, "news")
	b.Subscribe(c, "news")
	b.Subscribe(c, "otro")

	if n := b.Publish("news", []byte("hola")); n != 2 {
		t.Fatalf("Publish news → %d receptores, quería 2", n)
	}
	if n := b.Publish("otro", []byte("x")); n != 1 {
		t.Fatalf("Publish otro → %d, quería 1", n)
	}
	if n := b.Publish("vacio", []byte("x")); n != 0 {
		t.Fatalf("Publish a canal sin subs → %d, quería 0", n)
	}
	if got := a.list(); len(got) != 1 || got[0] != "news:hola" {
		t.Fatalf("a recibió %v", got)
	}
	if got := c.list(); len(got) != 1 || got[0] != "news:hola" {
		t.Fatalf("c recibió %v", got)
	}
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	b := New()
	a := &recorder{}
	b.Subscribe(a, "news")
	if !b.Unsubscribe(a, "news") {
		t.Fatal("Unsubscribe de canal suscrito → false")
	}
	if b.Unsubscribe(a, "news") {
		t.Fatal("Unsubscribe repetido → true")
	}
	if n := b.Publish("news", []byte("hola")); n != 0 {
		t.Fatalf("Publish tras unsubscribe → %d, quería 0", n)
	}
	if got := a.list(); len(got) != 0 {
		t.Fatalf("a no debería haber recibido nada: %v", got)
	}
}

func TestSubscribeIdempotent(t *testing.T) {
	b := New()
	a := &recorder{}
	if !b.Subscribe(a, "news") {
		t.Fatal("primera Subscribe → false")
	}
	if b.Subscribe(a, "news") {
		t.Fatal("Subscribe repetida → true")
	}
	if n := b.Publish("news", []byte("x")); n != 1 {
		t.Fatalf("Publish → %d, quería 1 (no duplicado)", n)
	}
}
```

- [ ] **Step 2: Ejecutar y verificar que falla** — `go test ./internal/pubsub/ -v` → FALLA (undefined: New, etc.).

- [ ] **Step 3: Implementar `internal/pubsub/broker.go`:**
```go
// Package pubsub enruta mensajes publish/subscribe entre canales y suscriptores.
package pubsub

import "sync"

// Subscriber recibe mensajes de los canales a los que está suscrito.
// La implementación de Deliver no debe entrar en pánico ni asumir orden global.
type Subscriber interface {
	Deliver(channel string, payload []byte)
}

// Broker mantiene, por canal, el conjunto de suscriptores.
type Broker struct {
	mu   sync.RWMutex
	subs map[string]map[Subscriber]struct{}
}

// New crea un broker vacío.
func New() *Broker {
	return &Broker{subs: make(map[string]map[Subscriber]struct{})}
}

// Subscribe añade sub al canal. Devuelve true si no estaba ya suscrito.
func (b *Broker) Subscribe(sub Subscriber, channel string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	set := b.subs[channel]
	if set == nil {
		set = make(map[Subscriber]struct{})
		b.subs[channel] = set
	}
	if _, ok := set[sub]; ok {
		return false
	}
	set[sub] = struct{}{}
	return true
}

// Unsubscribe quita sub del canal. Devuelve true si estaba suscrito.
func (b *Broker) Unsubscribe(sub Subscriber, channel string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	set := b.subs[channel]
	if set == nil {
		return false
	}
	if _, ok := set[sub]; !ok {
		return false
	}
	delete(set, sub)
	if len(set) == 0 {
		delete(b.subs, channel)
	}
	return true
}

// Publish entrega payload a los suscriptores del canal y devuelve cuántos eran.
// Toma una instantánea bajo lock y entrega fuera del lock para no bloquear el
// broker mientras escribe a cada suscriptor.
func (b *Broker) Publish(channel string, payload []byte) int {
	b.mu.RLock()
	set := b.subs[channel]
	targets := make([]Subscriber, 0, len(set))
	for sub := range set {
		targets = append(targets, sub)
	}
	b.mu.RUnlock()

	for _, sub := range targets {
		sub.Deliver(channel, payload)
	}
	return len(targets)
}
```

- [ ] **Step 4: Ejecutar y verificar que pasa (con -race)** — `go test ./internal/pubsub/ -race -v` → PASS, sin races.

- [ ] **Step 5: Commit**

```bash
git add internal/pubsub/
git -c user.email="developert@orvixapp.com" -c user.name="cesar" commit -m "feat: broker pub/sub (Subscribe/Unsubscribe/Publish)"
```

---

### Task 2: Cliente y comandos pub/sub en el servidor

**Files:**
- Create: `internal/server/pubsub.go`
- Modify: `internal/server/server.go`
- Modify: `internal/server/server_test.go`

- [ ] **Step 1: Añadir el test que falla** — APPEND a `internal/server/server_test.go` (y añadir `"io"` y `"strconv"` al bloque de imports si no están):
```go
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
	return []string{line}
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
```

- [ ] **Step 2: Ejecutar y verificar que falla** — `go test ./internal/server/ -run TestPubSub -v` → FALLA: el servidor no conoce SUBSCRIBE/PUBLISH (devuelve `-ERR comando desconocido`), así que `readReply` no obtiene el array esperado.

- [ ] **Step 3: Crear `internal/server/pubsub.go`:**
```go
package server

import (
	"net"
	"sync"

	"llavero/internal/protocol"
)

// client representa una conexión: escritura serializada + sus suscripciones.
// El mapa subs solo se accede desde la goroutine de la conexión.
type client struct {
	conn    net.Conn
	proto   protocol.Protocol
	writeMu sync.Mutex
	subs    map[string]struct{}
}

func newClient(conn net.Conn, proto protocol.Protocol) *client {
	return &client{conn: conn, proto: proto, subs: make(map[string]struct{})}
}

// send serializa la escritura de una respuesta en la conexión.
func (c *client) send(reply protocol.Reply) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.proto.Encode(c.conn, reply)
}

// Deliver implementa pubsub.Subscriber: empuja un mensaje al cliente.
func (c *client) Deliver(channel string, payload []byte) {
	_ = c.send(protocol.ArrayReply{Elems: []protocol.Reply{
		protocol.BulkReply{Value: []byte("message")},
		protocol.BulkReply{Value: []byte(channel)},
		protocol.BulkReply{Value: payload},
	}})
}

// pubSubReply construye la confirmación de (un)subscribe.
func pubSubReply(kind, channel string, count int) protocol.Reply {
	return protocol.ArrayReply{Elems: []protocol.Reply{
		protocol.BulkReply{Value: []byte(kind)},
		protocol.BulkReply{Value: []byte(channel)},
		protocol.IntReply{N: int64(count)},
	}}
}

// cmdSubscribe suscribe al cliente a los canales dados y confirma cada uno.
// Envía las respuestas directamente y devuelve nil.
func (s *Server) cmdSubscribe(c *client, args [][]byte) protocol.Reply {
	if len(args) < 1 {
		return protocol.ErrorReply{Msg: "ERR SUBSCRIBE requiere al menos 1 canal"}
	}
	for _, ch := range args {
		channel := string(ch)
		s.broker.Subscribe(c, channel)
		c.subs[channel] = struct{}{}
		_ = c.send(pubSubReply("subscribe", channel, len(c.subs)))
	}
	return nil
}

// cmdUnsubscribe da de baja al cliente de los canales dados (o de todos si no
// se pasan). Envía las respuestas directamente y devuelve nil.
func (s *Server) cmdUnsubscribe(c *client, args [][]byte) protocol.Reply {
	channels := make([]string, 0, len(args))
	if len(args) == 0 {
		for ch := range c.subs {
			channels = append(channels, ch)
		}
	} else {
		for _, ch := range args {
			channels = append(channels, string(ch))
		}
	}
	if len(channels) == 0 {
		_ = c.send(protocol.ArrayReply{Elems: []protocol.Reply{
			protocol.BulkReply{Value: []byte("unsubscribe")},
			protocol.BulkReply{Null: true},
			protocol.IntReply{N: 0},
		}})
		return nil
	}
	for _, channel := range channels {
		s.broker.Unsubscribe(c, channel)
		delete(c.subs, channel)
		_ = c.send(pubSubReply("unsubscribe", channel, len(c.subs)))
	}
	return nil
}

// cmdPublish entrega un mensaje a los suscriptores del canal.
func (s *Server) cmdPublish(args [][]byte) protocol.Reply {
	if len(args) != 2 {
		return protocol.ErrorReply{Msg: "ERR PUBLISH requiere canal y mensaje"}
	}
	n := s.broker.Publish(string(args[0]), args[1])
	return protocol.IntReply{N: int64(n)}
}

// unsubscribeAll da de baja al cliente de todos sus canales (al desconectar).
func (s *Server) unsubscribeAll(c *client) {
	for ch := range c.subs {
		s.broker.Unsubscribe(c, ch)
	}
}
```

- [ ] **Step 4: Modificar `internal/server/server.go`**

(a) Añadir el import de pubsub y `strings`. El bloque de imports actual es:
```go
import (
	"bufio"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"llavero/internal/command"
	"llavero/internal/persistence"
	"llavero/internal/protocol"
	"llavero/internal/store"
)
```
Reemplazarlo por:
```go
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
```

(b) Añadir el campo `broker` al struct `Server` (tras el campo `proto protocol.Protocol`):
```go
	broker       *pubsub.Broker
```

(c) En `NewWithOptions`, en el literal `&Server{...}`, añadir la inicialización del broker (tras `proto: protocol.RESP{},`):
```go
		broker:       pubsub.New(),
```

(d) Reemplazar por completo la función `handleConn` actual por:
```go
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
```

- [ ] **Step 5: Ejecutar y verificar que pasa (con -race)** — `go test ./internal/server/ -race -v` → PASS en todos (los previos + TestPubSub). El test pub/sub ejercita entrega entre dos conexiones bajo `-race`.

- [ ] **Step 6: Suite completa** — `go test ./... -race` → todos los paquetes en verde.

- [ ] **Step 7: Commit**

```bash
git add internal/server/
git -c user.email="developert@orvixapp.com" -c user.name="cesar" commit -m "feat: comandos SUBSCRIBE/UNSUBSCRIBE/PUBLISH con entrega a suscriptores"
```

---

### Task 3: Verificación final de la fase

**Files:** ninguno (solo verificación).

- [ ] **Step 1: vet + build + suite completa con -race**

Run: `go vet ./... && go build ./... && go test ./... -race`
Expected: vet sin avisos, build OK, todos los paquetes en verde.

- [ ] **Step 2: Prueba de humo manual (si el entorno lo permite)**

Si hay `redis-cli`, en una terminal `redis-cli -p 6380 SUBSCRIBE news` y en otra `redis-cli -p 6380 PUBLISH news hola`; el suscriptor debe imprimir el mensaje. En este sandbox los clientes de red en segundo plano pueden no capturar stdout; en ese caso la fase queda verificada por el broker (tests unitarios) y `TestPubSub` (entrega entre dos conexiones TCP reales bajo `-race`). No inventar resultados de smoke que no se puedan observar.

---

## Resultado de la fase

Al terminar la Fase 9, Llavero soporta pub/sub (`SUBSCRIBE`/`UNSUBSCRIBE`/`PUBLISH`)
con entrega de mensajes a las conexiones suscritas y baja automática al
desconectar. La Fase 10 añadirá transacciones (`MULTI`/`EXEC`/`DISCARD`).
