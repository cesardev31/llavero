# Llavero Fase 7 — Plan de Implementación (Comandos string/admin)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Añadir comandos string (`INCR`/`DECR`/`INCRBY`/`DECRBY`/`MSET`/`MGET`/`SETNX`) y admin (`TYPE`/`KEYS`/`DBSIZE`/`FLUSHALL`) para que Llavero sea mucho más práctico.

**Architecture:** Las operaciones nuevas del store van en archivos nuevos (`string.go`, `admin.go`) para no engordar `store.go`, reutilizando el helper `liveEntry` y `ErrWrongType`. Los handlers van en `handlers_string.go` y `handlers_admin.go`, registrados en `command.go`.

**Tech Stack:** Go 1.26, librería estándar (`errors`, `strconv`, `path`, `time`). Sin dependencias externas.

## Semántica (estilo Redis)

- `INCR/DECR k` y `INCRBY/DECRBY k n`: suman sobre el valor entero. Clave ausente = 0. `ErrWrongType` si no es string; `ErrNotInteger` si el string no es un entero. Preservan el TTL existente. Devuelven `:resultado`.
- `MSET k v [k v ...]` → `+OK` (cada clave se fija atómicamente; NO hay atomicidad global entre shards).
- `MGET k [k ...]` → array; bulk nulo para claves ausentes o de otro tipo (nunca error).
- `SETNX k v` → `:1` si la fijó, `:0` si ya existía.
- `TYPE k` → `+string`/`+list`/`+hash`/`+set`/`+none`.
- `KEYS patrón` → array de claves vivas que casan (glob `path.Match`: `*`, `?`, `[abc]`; `*` no cruza `/`).
- `DBSIZE` → `:n` claves vivas. `FLUSHALL` → `+OK`, borra todo en memoria.

## Estructura de archivos

- `internal/store/string.go` + `string_test.go` — `IncrBy`, `SetNX`, `ErrNotInteger`.
- `internal/store/admin.go` + `admin_test.go` — `Type`, `Keys`, `Len`, `Flush`.
- `internal/command/handlers_string.go` — handlers de contadores/MSET/MGET/SETNX.
- `internal/command/handlers_admin.go` — handlers TYPE/KEYS/DBSIZE/FLUSHALL.
- `internal/command/command.go` — registrar los 11 comandos.
- `internal/command/command_test.go` — tests de los comandos nuevos.

---

### Task 1: Operaciones string del store (`internal/store/string.go`)

**Files:**
- Create: `internal/store/string.go`
- Create: `internal/store/string_test.go`

- [ ] **Step 1: Escribir los tests que fallan** — crear `internal/store/string_test.go`:
```go
package store

import "testing"

func TestIncrBy(t *testing.T) {
	s := New(16)
	// clave inexistente cuenta como 0
	if n, err := s.IncrBy("c", 1); err != nil || n != 1 {
		t.Fatalf("IncrBy nueva → %d, %v", n, err)
	}
	if n, err := s.IncrBy("c", 5); err != nil || n != 6 {
		t.Fatalf("IncrBy → %d, %v", n, err)
	}
	if n, err := s.IncrBy("c", -2); err != nil || n != 4 {
		t.Fatalf("IncrBy negativo → %d, %v", n, err)
	}
	// el valor quedó como string "4"
	if v, ok, _ := s.Get("c"); !ok || string(v) != "4" {
		t.Fatalf("Get tras IncrBy → %q %v", v, ok)
	}
}

func TestIncrByErrors(t *testing.T) {
	s := New(16)
	s.Set("texto", []byte("abc"))
	if _, err := s.IncrBy("texto", 1); err != ErrNotInteger {
		t.Fatalf("IncrBy sobre no-entero → %v, quería ErrNotInteger", err)
	}
	s.RPush("lista", []byte("x"))
	if _, err := s.IncrBy("lista", 1); err != ErrWrongType {
		t.Fatalf("IncrBy sobre lista → %v, quería ErrWrongType", err)
	}
}

func TestIncrByPreservesTTL(t *testing.T) {
	s := New(16)
	s.Set("c", []byte("1"))
	s.Expire("c", 100*time.Second)
	if _, err := s.IncrBy("c", 1); err != nil {
		t.Fatalf("IncrBy → %v", err)
	}
	if _, _, hasExpiry := s.TTL("c"); !hasExpiry {
		t.Fatal("IncrBy perdió el TTL")
	}
}

func TestSetNX(t *testing.T) {
	s := New(16)
	if !s.SetNX("k", []byte("v1")) {
		t.Fatal("SetNX en clave nueva → false")
	}
	if s.SetNX("k", []byte("v2")) {
		t.Fatal("SetNX en clave existente → true")
	}
	if v, ok, _ := s.Get("k"); !ok || string(v) != "v1" {
		t.Fatalf("valor tras SetNX → %q %v", v, ok)
	}
}
```

Nota: `string_test.go` usa `time` (en TestIncrByPreservesTTL); incluir `"time"` en sus imports junto a `"testing"`.

- [ ] **Step 2: Ejecutar y verificar que falla** — `go test ./internal/store/ -run 'TestIncrBy|TestSetNX' -v` → FALLA (IncrBy/SetNX/ErrNotInteger undefined).

- [ ] **Step 3: Implementar `internal/store/string.go`:**
```go
package store

import (
	"errors"
	"strconv"
	"time"
)

// ErrNotInteger se devuelve cuando un valor string no representa un entero.
var ErrNotInteger = errors.New("ERR value is not an integer or out of range")

// IncrBy suma delta al valor entero de key y devuelve el resultado. Una clave
// inexistente cuenta como 0. Preserva el TTL existente. ErrWrongType si la
// clave no es string; ErrNotInteger si el valor no es un entero válido.
func (s *Store) IncrBy(key string, delta int64) (int64, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.liveEntry(key, time.Now())
	var n int64
	if ok {
		b, isStr := e.value.([]byte)
		if !isStr {
			return 0, ErrWrongType
		}
		parsed, err := strconv.ParseInt(string(b), 10, 64)
		if err != nil {
			return 0, ErrNotInteger
		}
		n = parsed
	} else {
		e = &entry{}
		sh.data[key] = e
	}
	n += delta
	e.value = []byte(strconv.FormatInt(n, 10))
	return n, nil
}

// SetNX fija el valor solo si la clave no existe. Devuelve true si la fijó.
func (s *Store) SetNX(key string, val []byte) bool {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	if _, ok := sh.liveEntry(key, time.Now()); ok {
		return false
	}
	sh.data[key] = &entry{value: val}
	return true
}
```

- [ ] **Step 4: Ejecutar y verificar que pasa (con -race)** — `go test ./internal/store/ -race -v` → PASS (todos, previos + nuevos), sin races.

- [ ] **Step 5: Commit**

```bash
git add internal/store/string.go internal/store/string_test.go
git -c user.email="developert@orvixapp.com" -c user.name="cesar" commit -m "feat: store IncrBy y SetNX para comandos de contador"
```

---

### Task 2: Operaciones admin del store (`internal/store/admin.go`)

**Files:**
- Create: `internal/store/admin.go`
- Create: `internal/store/admin_test.go`

- [ ] **Step 1: Escribir los tests que fallan** — crear `internal/store/admin_test.go`:
```go
package store

import (
	"sort"
	"testing"
	"time"
)

func TestType(t *testing.T) {
	s := New(16)
	s.Set("str", []byte("v"))
	s.RPush("lst", []byte("a"))
	s.HSet("hsh", "f", []byte("v"))
	s.SAdd("set", []byte("m"))
	cases := map[string]string{"str": "string", "lst": "list", "hsh": "hash", "set": "set", "nope": "none"}
	for key, want := range cases {
		if got := s.Type(key); got != want {
			t.Errorf("Type(%q) = %q, quería %q", key, got, want)
		}
	}
}

func TestKeysAndLen(t *testing.T) {
	s := New(16)
	s.Set("user:1", []byte("a"))
	s.Set("user:2", []byte("b"))
	s.Set("otro", []byte("c"))
	if n := s.Len(); n != 3 {
		t.Fatalf("Len → %d, quería 3", n)
	}
	got, err := s.Keys("user:*")
	if err != nil {
		t.Fatalf("Keys error: %v", err)
	}
	strs := make([]string, len(got))
	for i, b := range got {
		strs[i] = string(b)
	}
	sort.Strings(strs)
	if len(strs) != 2 || strs[0] != "user:1" || strs[1] != "user:2" {
		t.Fatalf("Keys user:* → %v", strs)
	}
	all, _ := s.Keys("*")
	if len(all) != 3 {
		t.Fatalf("Keys * → %d, quería 3", len(all))
	}
}

func TestKeysSkipsExpired(t *testing.T) {
	s := New(16)
	s.Set("vivo", []byte("a"))
	s.Set("muerto", []byte("b"))
	s.Expire("muerto", -time.Second)
	if n := s.Len(); n != 1 {
		t.Fatalf("Len con una vencida → %d, quería 1", n)
	}
}

func TestFlush(t *testing.T) {
	s := New(16)
	s.Set("a", []byte("1"))
	s.Set("b", []byte("2"))
	s.Flush()
	if n := s.Len(); n != 0 {
		t.Fatalf("Len tras Flush → %d, quería 0", n)
	}
}
```

- [ ] **Step 2: Ejecutar y verificar que falla** — `go test ./internal/store/ -run 'TestType|TestKeys|TestFlush' -v` → FALLA (Type/Keys/Len/Flush undefined).

- [ ] **Step 3: Implementar `internal/store/admin.go`:**
```go
package store

import (
	"path"
	"time"
)

// Type devuelve el tipo de la clave: "string", "list", "hash", "set" o "none".
func (s *Store) Type(key string) string {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.liveEntry(key, time.Now())
	if !ok {
		return "none"
	}
	switch e.value.(type) {
	case []byte:
		return "string"
	case [][]byte:
		return "list"
	case map[string][]byte:
		return "hash"
	case map[string]struct{}:
		return "set"
	default:
		return "none"
	}
}

// Keys devuelve las claves vivas que casan con el patrón glob (*, ?, [abc]).
func (s *Store) Keys(pattern string) ([][]byte, error) {
	now := time.Now()
	out := [][]byte{}
	for _, sh := range s.shards {
		sh.mu.RLock()
		for k, e := range sh.data {
			if e.expired(now) {
				continue
			}
			match, err := path.Match(pattern, k)
			if err != nil {
				sh.mu.RUnlock()
				return nil, err
			}
			if match {
				out = append(out, []byte(k))
			}
		}
		sh.mu.RUnlock()
	}
	return out, nil
}

// Len devuelve el número de claves vivas.
func (s *Store) Len() int {
	now := time.Now()
	n := 0
	for _, sh := range s.shards {
		sh.mu.RLock()
		for _, e := range sh.data {
			if !e.expired(now) {
				n++
			}
		}
		sh.mu.RUnlock()
	}
	return n
}

// Flush borra todas las claves de todos los shards.
func (s *Store) Flush() {
	for _, sh := range s.shards {
		sh.mu.Lock()
		sh.data = make(map[string]*entry)
		sh.mu.Unlock()
	}
}
```

- [ ] **Step 4: Ejecutar y verificar que pasa (con -race)** — `go test ./internal/store/ -race -v` → PASS, sin races.

- [ ] **Step 5: Commit**

```bash
git add internal/store/admin.go internal/store/admin_test.go
git -c user.email="developert@orvixapp.com" -c user.name="cesar" commit -m "feat: store Type/Keys/Len/Flush para comandos admin"
```

---

### Task 3: Handlers y registro de comandos (`internal/command`)

**Files:**
- Create: `internal/command/handlers_string.go`
- Create: `internal/command/handlers_admin.go`
- Modify: `internal/command/command.go` (registrar)
- Modify: `internal/command/command_test.go` (tests)

- [ ] **Step 1: Añadir los tests que fallan** — APPEND a `internal/command/command_test.go`:
```go
func TestCounterCommands(t *testing.T) {
	d := NewDispatcher()
	s := store.New(16)
	if r := dispatch(d, s, "INCR", "c"); r != (protocol.IntReply{N: 1}) {
		t.Fatalf("INCR → %#v", r)
	}
	if r := dispatch(d, s, "INCRBY", "c", "9"); r != (protocol.IntReply{N: 10}) {
		t.Fatalf("INCRBY → %#v", r)
	}
	if r := dispatch(d, s, "DECR", "c"); r != (protocol.IntReply{N: 9}) {
		t.Fatalf("DECR → %#v", r)
	}
	if r := dispatch(d, s, "DECRBY", "c", "4"); r != (protocol.IntReply{N: 5}) {
		t.Fatalf("DECRBY → %#v", r)
	}
	dispatch(d, s, "SET", "txt", "abc")
	if _, ok := dispatch(d, s, "INCR", "txt").(protocol.ErrorReply); !ok {
		t.Error("INCR sobre no-entero debería dar ErrorReply")
	}
}

func TestMSetMGetSetNX(t *testing.T) {
	d := NewDispatcher()
	s := store.New(16)
	if r := dispatch(d, s, "MSET", "a", "1", "b", "2"); r != (protocol.StatusReply{Msg: "OK"}) {
		t.Fatalf("MSET → %#v", r)
	}
	r := dispatch(d, s, "MGET", "a", "nope", "b")
	arr, ok := r.(protocol.ArrayReply)
	if !ok || len(arr.Elems) != 3 {
		t.Fatalf("MGET → %#v", r)
	}
	if b, ok := arr.Elems[0].(protocol.BulkReply); !ok || string(b.Value) != "1" {
		t.Fatalf("MGET[0] → %#v", arr.Elems[0])
	}
	if b, ok := arr.Elems[1].(protocol.BulkReply); !ok || !b.Null {
		t.Fatalf("MGET[1] (ausente) debería ser nulo → %#v", arr.Elems[1])
	}
	if r := dispatch(d, s, "SETNX", "a", "x"); r != (protocol.IntReply{N: 0}) {
		t.Fatalf("SETNX existente → %#v", r)
	}
	if r := dispatch(d, s, "SETNX", "nueva", "x"); r != (protocol.IntReply{N: 1}) {
		t.Fatalf("SETNX nueva → %#v", r)
	}
}

func TestAdminCommands(t *testing.T) {
	d := NewDispatcher()
	s := store.New(16)
	dispatch(d, s, "SET", "k", "v")
	if r := dispatch(d, s, "TYPE", "k"); r != (protocol.StatusReply{Msg: "string"}) {
		t.Fatalf("TYPE → %#v", r)
	}
	if r := dispatch(d, s, "TYPE", "nope"); r != (protocol.StatusReply{Msg: "none"}) {
		t.Fatalf("TYPE inexistente → %#v", r)
	}
	if r := dispatch(d, s, "DBSIZE"); r != (protocol.IntReply{N: 1}) {
		t.Fatalf("DBSIZE → %#v", r)
	}
	r := dispatch(d, s, "KEYS", "*")
	if arr, ok := r.(protocol.ArrayReply); !ok || len(arr.Elems) != 1 {
		t.Fatalf("KEYS * → %#v", r)
	}
	if r := dispatch(d, s, "FLUSHALL"); r != (protocol.StatusReply{Msg: "OK"}) {
		t.Fatalf("FLUSHALL → %#v", r)
	}
	if r := dispatch(d, s, "DBSIZE"); r != (protocol.IntReply{N: 0}) {
		t.Fatalf("DBSIZE tras FLUSHALL → %#v", r)
	}
}
```

- [ ] **Step 2: Ejecutar y verificar que falla** — `go test ./internal/command/ -run 'TestCounterCommands|TestMSetMGetSetNX|TestAdminCommands' -v` → FALLA (comandos desconocidos → ErrorReply, comparaciones fallan).

- [ ] **Step 3: Crear `internal/command/handlers_string.go`:**
```go
package command

import (
	"strconv"

	"llavero/internal/protocol"
	"llavero/internal/store"
)

func incrByReply(s *store.Store, key string, delta int64) protocol.Reply {
	n, err := s.IncrBy(key, delta)
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	return protocol.IntReply{N: n}
}

func cmdIncr(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR INCR requiere 1 argumento"}
	}
	return incrByReply(s, string(args[0]), 1)
}

func cmdDecr(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR DECR requiere 1 argumento"}
	}
	return incrByReply(s, string(args[0]), -1)
}

func cmdIncrBy(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 2 {
		return protocol.ErrorReply{Msg: "ERR INCRBY requiere 2 argumentos"}
	}
	delta, err := strconv.ParseInt(string(args[1]), 10, 64)
	if err != nil {
		return protocol.ErrorReply{Msg: "ERR el incremento debe ser un entero"}
	}
	return incrByReply(s, string(args[0]), delta)
}

func cmdDecrBy(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 2 {
		return protocol.ErrorReply{Msg: "ERR DECRBY requiere 2 argumentos"}
	}
	delta, err := strconv.ParseInt(string(args[1]), 10, 64)
	if err != nil {
		return protocol.ErrorReply{Msg: "ERR el decremento debe ser un entero"}
	}
	return incrByReply(s, string(args[0]), -delta)
}

func cmdMSet(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) < 2 || len(args)%2 != 0 {
		return protocol.ErrorReply{Msg: "ERR MSET requiere pares clave valor"}
	}
	for i := 0; i < len(args); i += 2 {
		s.Set(string(args[i]), args[i+1])
	}
	return protocol.StatusReply{Msg: "OK"}
}

func cmdMGet(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) < 1 {
		return protocol.ErrorReply{Msg: "ERR MGET requiere al menos 1 clave"}
	}
	elems := make([]protocol.Reply, len(args))
	for i, a := range args {
		v, ok, err := s.Get(string(a))
		if err != nil || !ok {
			elems[i] = protocol.BulkReply{Null: true}
		} else {
			elems[i] = protocol.BulkReply{Value: v}
		}
	}
	return protocol.ArrayReply{Elems: elems}
}

func cmdSetNX(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 2 {
		return protocol.ErrorReply{Msg: "ERR SETNX requiere 2 argumentos"}
	}
	if s.SetNX(string(args[0]), args[1]) {
		return protocol.IntReply{N: 1}
	}
	return protocol.IntReply{N: 0}
}
```

- [ ] **Step 4: Crear `internal/command/handlers_admin.go`:**
```go
package command

import (
	"llavero/internal/protocol"
	"llavero/internal/store"
)

func cmdType(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR TYPE requiere 1 argumento"}
	}
	return protocol.StatusReply{Msg: s.Type(string(args[0]))}
}

func cmdKeys(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR KEYS requiere 1 patrón"}
	}
	items, err := s.Keys(string(args[0]))
	if err != nil {
		return protocol.ErrorReply{Msg: "ERR patrón inválido"}
	}
	return bulkArray(items)
}

func cmdDBSize(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 0 {
		return protocol.ErrorReply{Msg: "ERR DBSIZE no acepta argumentos"}
	}
	return protocol.IntReply{N: int64(s.Len())}
}

func cmdFlushAll(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 0 {
		return protocol.ErrorReply{Msg: "ERR FLUSHALL no acepta argumentos"}
	}
	s.Flush()
	return protocol.StatusReply{Msg: "OK"}
}
```

- [ ] **Step 5: Registrar en `internal/command/command.go`** — tras la línea `d.handlers["SCARD"] = cmdSCard`, añadir:
```go
	d.handlers["INCR"] = cmdIncr
	d.handlers["DECR"] = cmdDecr
	d.handlers["INCRBY"] = cmdIncrBy
	d.handlers["DECRBY"] = cmdDecrBy
	d.handlers["MSET"] = cmdMSet
	d.handlers["MGET"] = cmdMGet
	d.handlers["SETNX"] = cmdSetNX
	d.handlers["TYPE"] = cmdType
	d.handlers["KEYS"] = cmdKeys
	d.handlers["DBSIZE"] = cmdDBSize
	d.handlers["FLUSHALL"] = cmdFlushAll
```

- [ ] **Step 6: Ejecutar y verificar que pasa (con -race)** — `go test ./internal/command/ ./internal/store/ -race -v` → PASS (previos + 3 nuevos tests de comandos).

- [ ] **Step 7: Commit**

```bash
git add internal/command/
git -c user.email="developert@orvixapp.com" -c user.name="cesar" commit -m "feat: comandos INCR/DECR/INCRBY/DECRBY/MSET/MGET/SETNX/TYPE/KEYS/DBSIZE/FLUSHALL"
```

---

### Task 4: Verificación final de la fase

**Files:** ninguno (solo verificación).

- [ ] **Step 1: vet + build + suite completa con -race**

Run: `go vet ./... && go build ./... && go test ./... -race`
Expected: vet sin avisos, build OK, todos los paquetes en verde.

- [ ] **Step 2: Prueba de humo manual**

Liberar el puerto 6380 si hace falta (`ss -ltnp | grep 6380`, matar por PID). Luego construir el binario y arrancarlo en segundo plano (`go build -o /tmp/llavero-bin ./cmd/llavero`), y con un cliente mini-RESP mínimo en Go (igual que en fases previas: enviar `*N\n$len\n...`) probar la secuencia y comparar:
```
INCR visitas        -> :1
INCRBY visitas 9    -> :10
SET nombre cesar    -> +OK
TYPE nombre         -> +string
TYPE visitas        -> +string
MSET a 1 b 2        -> +OK
MGET a nope b       -> array(3): $1 (nil) $2
SETNX a 9           -> :0
DBSIZE              -> :4
KEYS *              -> array(4): ... (orden no garantizado)
FLUSHALL            -> +OK
DBSIZE              -> :0
```
Al terminar, parar el servidor (kill por PID) y borrar los temporales.

Nota: en este entorno los clientes de red en segundo plano pueden no mostrar stdout; si pasa, basta con confirmar la fase vía la suite de tests (`go test ./... -race`), que cubre todos los comandos a nivel de dispatcher.

---

## Resultado de la fase

Al terminar la Fase 7, Llavero soporta contadores atómicos, set/get múltiples,
SETNX e introspección/admin (TYPE/KEYS/DBSIZE/FLUSHALL). La Fase 8 implementará
la capa RESP real para usar `redis-cli`/`go-redis`.
