# Llavero Fase 4 — Plan de Implementación (Estructuras: listas, hashes, sets)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Que Llavero soporte listas, hashes y sets, con respuestas de array en el protocolo y errores WRONGTYPE al mezclar tipos.

**Architecture:** `entry.value` pasa de `[]byte` a `any` (string=`[]byte`, lista=`[][]byte`, hash=`map[string][]byte`, set=`map[string]struct{}`). Las operaciones por tipo viven en el store (atómicas bajo el lock del shard), repartidas en `list.go`/`hash.go`/`set.go`. El protocolo gana `ArrayReply`. Los handlers se reparten en `handlers_list.go`/`handlers_hash.go`/`handlers_set.go`.

**Tech Stack:** Go 1.26, librería estándar (`errors`, `strconv`, `time`, `sync`, `hash/fnv`). Sin dependencias externas.

## Semántica (estilo Redis)

- Tipos mezclados → error `-WRONGTYPE ...`.
- `LPUSH/RPUSH key v...` → `:longitud`. `LPOP/RPOP key` → bulk o nulo. `LLEN` → `:n`. `LRANGE key ini fin` → array (índices negativos soportados).
- `HSET key campo valor` → `:1` si el campo es nuevo, `:0` si se actualizó. `HGET` → bulk o nulo. `HDEL key campo...` → `:n`. `HGETALL` → array plano (campo,valor,...). `HLEN` → `:n`.
- `SADD key m...` → `:añadidos`. `SREM` → `:n`. `SISMEMBER` → `:0/1`. `SMEMBERS` → array. `SCARD` → `:n`.
- Listas/hashes/sets que quedan vacíos borran la clave. Lecturas sobre clave inexistente → array vacío / nulo / 0 según el comando.

## Estructura de archivos

- `internal/protocol/protocol.go` + `miniresp.go` + `miniresp_test.go` — añadir `ArrayReply`.
- `internal/store/store.go` — `entry.value any`, `ErrWrongType`, helper `liveEntry`, `Get` con tipo, `Exists` reescrito.
- `internal/store/store_test.go` — actualizar llamadas a `Get` (ahora 3 retornos).
- `internal/store/list.go` + `list_test.go` — operaciones de lista.
- `internal/store/hash.go` + `hash_test.go` — operaciones de hash.
- `internal/store/set.go` + `set_test.go` — operaciones de set.
- `internal/command/handlers.go` — `cmdGet` actualizado + helper `bulkArray`.
- `internal/command/handlers_list.go` / `handlers_hash.go` / `handlers_set.go` — handlers.
- `internal/command/command.go` — registrar los 16 comandos.
- `internal/command/command_test.go` — tests de los comandos por estructura.

---

### Task 1: ArrayReply en el protocolo

**Files:**
- Modify: `internal/protocol/protocol.go`
- Modify: `internal/protocol/miniresp.go`
- Modify: `internal/protocol/miniresp_test.go`

- [ ] **Step 1: Añadir el test que falla**

APPEND a `internal/protocol/miniresp_test.go`:
```go
func TestEncodeArrayReply(t *testing.T) {
	var buf bytes.Buffer
	reply := ArrayReply{Elems: []Reply{
		BulkReply{Value: []byte("a")},
		BulkReply{Value: []byte("bb")},
	}}
	if err := (MiniRESP{}).Encode(&buf, reply); err != nil {
		t.Fatalf("Encode error: %v", err)
	}
	if want := "*2\n$1\na\n$2\nbb\n"; buf.String() != want {
		t.Errorf("Encode array = %q, quería %q", buf.String(), want)
	}

	buf.Reset()
	if err := (MiniRESP{}).Encode(&buf, ArrayReply{}); err != nil {
		t.Fatalf("Encode vacío error: %v", err)
	}
	if buf.String() != "*0\n" {
		t.Errorf("array vacío = %q, quería \"*0\\n\"", buf.String())
	}
}
```

- [ ] **Step 2: Ejecutar y verificar que falla**

Run: `go test ./internal/protocol/ -run TestEncodeArrayReply -v`
Expected: FALLA al compilar con "undefined: ArrayReply".

- [ ] **Step 3: Añadir el tipo en `internal/protocol/protocol.go`**

Tras el bloque de tipos Reply existentes (después de `BulkReply`), añadir:
```go
// ArrayReply es una respuesta de array (p.ej. LRANGE, SMEMBERS). Sus elementos
// suelen ser BulkReply, pero puede anidar cualquier Reply.
type ArrayReply struct{ Elems []Reply }
```
Y junto a los demás métodos `isReply`, añadir:
```go
func (ArrayReply) isReply() {}
```

- [ ] **Step 4: Añadir el caso en el `Encode` de `internal/protocol/miniresp.go`**

En el `switch r := reply.(type)` de `Encode`, antes del `default`, añadir:
```go
	case ArrayReply:
		if _, err := fmt.Fprintf(w, "*%d\n", len(r.Elems)); err != nil {
			return err
		}
		for _, el := range r.Elems {
			if err := (MiniRESP{}).Encode(w, el); err != nil {
				return err
			}
		}
		return nil
```

- [ ] **Step 5: Ejecutar y verificar que pasa**

Run: `go test ./internal/protocol/ -v`
Expected: PASS en todos los tests.

- [ ] **Step 6: Commit**

```bash
git add internal/protocol/
git -c user.email="developert@orvixapp.com" -c user.name="cesar" commit -m "feat: ArrayReply en el protocolo mini-RESP"
```

---

### Task 2: Modelo de valores tipados + WRONGTYPE (`internal/store`)

**Files:**
- Modify (full rewrite): `internal/store/store.go`
- Modify: `internal/store/store_test.go` (actualizar llamadas a Get)
- Modify: `internal/command/handlers.go` (actualizar cmdGet)

- [ ] **Step 1: Reescribir `internal/store/store.go`**

Reemplazar TODO el contenido por:
```go
// Package store implementa un almacén clave-valor en memoria, dividido en
// shards independientes. Cada entrada guarda un valor de uno de varios tipos
// (string, lista, hash, set) y una expiración (TTL) opcional.
package store

import (
	"errors"
	"hash/fnv"
	"sync"
	"time"
)

// ErrWrongType se devuelve cuando una operación se aplica a una clave que
// contiene un tipo de valor distinto al esperado (estilo WRONGTYPE de Redis).
var ErrWrongType = errors.New("WRONGTYPE Operation against a key holding the wrong kind of value")

// entry es el valor almacenado. value contiene uno de:
//
//	[]byte (string) | [][]byte (lista) | map[string][]byte (hash) |
//	map[string]struct{} (set)
//
// Un expireAt cero significa "sin expiración".
type entry struct {
	value    any
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

// liveEntry devuelve la entrada viva de key, borrando perezosamente si venció.
// El caller debe tener tomado sh.mu en modo escritura (puede borrar).
func (sh *shard) liveEntry(key string, now time.Time) (*entry, bool) {
	e, ok := sh.data[key]
	if !ok {
		return nil, false
	}
	if e.expired(now) {
		delete(sh.data, key)
		return nil, false
	}
	return e, true
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

// Set guarda un valor string, limpiando cualquier TTL o tipo previo.
func (s *Store) Set(key string, val []byte) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.data[key] = &entry{value: val}
}

// Get devuelve el valor string de una clave. exists=false si no existe;
// error=ErrWrongType si la clave contiene un tipo que no es string.
func (s *Store) Get(key string) (val []byte, exists bool, err error) {
	sh := s.shardFor(key)
	now := time.Now()

	sh.mu.RLock()
	e, ok := sh.data[key]
	if ok && !e.expired(now) {
		b, isStr := e.value.([]byte)
		sh.mu.RUnlock()
		if !isStr {
			return nil, false, ErrWrongType
		}
		return b, true, nil
	}
	sh.mu.RUnlock()

	if ok {
		sh.mu.Lock()
		if e2, ok2 := sh.data[key]; ok2 && e2.expired(time.Now()) {
			delete(sh.data, key)
		}
		sh.mu.Unlock()
	}
	return nil, false, nil
}

// Exists indica si la clave existe (de cualquier tipo, con expiración perezosa).
func (s *Store) Exists(key string) bool {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	_, ok := sh.liveEntry(key, time.Now())
	return ok
}

// Del borra una clave y devuelve si existía de forma efectiva.
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

// Expire fija una expiración relativa para una clave existente.
func (s *Store) Expire(key string, d time.Duration) bool {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.liveEntry(key, time.Now())
	if !ok {
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
	e, ok := sh.liveEntry(key, time.Now())
	if !ok {
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
	now := time.Now()
	e, ok := sh.liveEntry(key, now)
	if !ok {
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
// tope de rondas. El orden aleatorio de iteración de mapas de Go da el muestreo.
func expireShard(sh *shard) {
	const maxRounds = 16
	now := time.Now()
	for round := 0; round < maxRounds; round++ {
		sh.mu.Lock()
		sampled, expired := 0, 0
		for key, e := range sh.data {
			if e.expireAt.IsZero() {
				continue
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
		if sampled == 0 || expired*4 < sampled {
			return
		}
	}
}
```

- [ ] **Step 2: Actualizar las llamadas a `Get` en `internal/store/store_test.go`**

`Get` ahora devuelve 3 valores. Actualizar cada llamada para descartar el tercero. Hacer estos reemplazos exactos:

1. `got, ok := s.Get("k")` → `got, ok, _ := s.Get("k")`
2. `if _, ok := s.Get("nope"); ok {` → `if _, ok, _ := s.Get("nope"); ok {`
3. `if _, ok := s.Get(key); !ok {` → `if _, ok, _ := s.Get(key); !ok {`
4. `if _, ok := s.Get("vivo"); !ok {` → `if _, ok, _ := s.Get("vivo"); !ok {`
5. `if _, ok := s.Get("futuro"); !ok {` → `if _, ok, _ := s.Get("futuro"); !ok {`
6. Las DOS apariciones de `if _, ok := s.Get("k"); ok {` → `if _, ok, _ := s.Get("k"); ok {` (reemplazar ambas, en TestLazyExpireOnGet y TestLazyExpireRemovesFromMap).

- [ ] **Step 3: Actualizar `cmdGet` en `internal/command/handlers.go`**

Reemplazar la función `cmdGet` por:
```go
func cmdGet(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR GET requiere 1 argumento"}
	}
	v, ok, err := s.Get(string(args[0]))
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	if !ok {
		return protocol.BulkReply{Null: true}
	}
	return protocol.BulkReply{Value: v}
}
```

- [ ] **Step 4: Ejecutar y verificar que pasa (con -race)**

Run: `go test ./... -race`
Expected: todos los paquetes en verde (store, command, protocol, server), sin data races. Los tests existentes siguen pasando con la nueva firma de `Get`.

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/store_test.go internal/command/handlers.go
git -c user.email="developert@orvixapp.com" -c user.name="cesar" commit -m "refactor: valores tipados en el store con ErrWrongType (Get tipado)"
```

---

### Task 3: Listas (store + comandos)

**Files:**
- Create: `internal/store/list.go`
- Create: `internal/store/list_test.go`
- Modify: `internal/command/handlers.go` (helper `bulkArray`)
- Create: `internal/command/handlers_list.go`
- Modify: `internal/command/command.go` (registrar comandos de lista)
- Modify: `internal/command/command_test.go` (test de comandos de lista)

- [ ] **Step 1: Añadir los tests de store que fallan** — crear `internal/store/list_test.go`:
```go
package store

import "testing"

func TestListPushPopLen(t *testing.T) {
	s := New(16)
	if n, err := s.RPush("l", []byte("a"), []byte("b")); err != nil || n != 2 {
		t.Fatalf("RPush → %d, %v", n, err)
	}
	if n, err := s.LPush("l", []byte("z")); err != nil || n != 3 {
		t.Fatalf("LPush → %d, %v", n, err)
	}
	// lista ahora: z a b
	if ln, _ := s.LLen("l"); ln != 3 {
		t.Fatalf("LLen → %d", ln)
	}
	if v, ok, _ := s.LPop("l"); !ok || string(v) != "z" {
		t.Fatalf("LPop → %q %v", v, ok)
	}
	if v, ok, _ := s.RPop("l"); !ok || string(v) != "b" {
		t.Fatalf("RPop → %q %v", v, ok)
	}
}

func TestListRange(t *testing.T) {
	s := New(16)
	s.RPush("l", []byte("a"), []byte("b"), []byte("c"), []byte("d"))
	if got, _ := s.LRange("l", 1, 2); len(got) != 2 || string(got[0]) != "b" || string(got[1]) != "c" {
		t.Fatalf("LRange 1 2 → %v", got)
	}
	if got, _ := s.LRange("l", 0, -1); len(got) != 4 {
		t.Fatalf("LRange 0 -1 → %v", got)
	}
	if got, _ := s.LRange("l", -2, -1); len(got) != 2 || string(got[0]) != "c" || string(got[1]) != "d" {
		t.Fatalf("LRange -2 -1 → %v", got)
	}
	if got, _ := s.LRange("vacia", 0, -1); len(got) != 0 {
		t.Fatalf("LRange inexistente → %v", got)
	}
}

func TestListWrongType(t *testing.T) {
	s := New(16)
	s.Set("str", []byte("v"))
	if _, err := s.RPush("str", []byte("x")); err != ErrWrongType {
		t.Fatalf("RPush sobre string → %v, quería ErrWrongType", err)
	}
	if _, err := s.LLen("str"); err != ErrWrongType {
		t.Fatalf("LLen sobre string → %v", err)
	}
}

func TestListEmptyDeletesKey(t *testing.T) {
	s := New(1)
	s.RPush("l", []byte("a"))
	s.LPop("l")
	if len(s.shards[0].data) != 0 {
		t.Fatal("la lista vacía no se borró")
	}
}
```

- [ ] **Step 2: Ejecutar y verificar que falla** — `go test ./internal/store/ -run TestList -v` → FALLA (RPush/LPush/... undefined).

- [ ] **Step 3: Implementar `internal/store/list.go`:**
```go
package store

import "time"

// listAt devuelve la lista viva de key. ok=false si no existe; ErrWrongType si
// la clave tiene otro tipo. El caller debe tener sh.mu tomado.
func (sh *shard) listAt(key string, now time.Time) ([][]byte, bool, error) {
	e, ok := sh.liveEntry(key, now)
	if !ok {
		return nil, false, nil
	}
	l, isList := e.value.([][]byte)
	if !isList {
		return nil, false, ErrWrongType
	}
	return l, true, nil
}

// LPush inserta valores por la izquierda (cabeza) y devuelve la longitud nueva.
func (s *Store) LPush(key string, vals ...[]byte) (int, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.liveEntry(key, time.Now())
	if !ok {
		e = &entry{value: [][]byte{}}
		sh.data[key] = e
	}
	list, isList := e.value.([][]byte)
	if !isList {
		return 0, ErrWrongType
	}
	for _, v := range vals {
		list = append([][]byte{v}, list...)
	}
	e.value = list
	return len(list), nil
}

// RPush inserta valores por la derecha (cola) y devuelve la longitud nueva.
func (s *Store) RPush(key string, vals ...[]byte) (int, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.liveEntry(key, time.Now())
	if !ok {
		e = &entry{value: [][]byte{}}
		sh.data[key] = e
	}
	list, isList := e.value.([][]byte)
	if !isList {
		return 0, ErrWrongType
	}
	list = append(list, vals...)
	e.value = list
	return len(list), nil
}

// LPop saca el primer elemento. ok=false si la lista no existe o está vacía.
func (s *Store) LPop(key string) ([]byte, bool, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.liveEntry(key, time.Now())
	if !ok {
		return nil, false, nil
	}
	list, isList := e.value.([][]byte)
	if !isList {
		return nil, false, ErrWrongType
	}
	if len(list) == 0 {
		return nil, false, nil
	}
	v := list[0]
	list = list[1:]
	if len(list) == 0 {
		delete(sh.data, key)
	} else {
		e.value = list
	}
	return v, true, nil
}

// RPop saca el último elemento. ok=false si la lista no existe o está vacía.
func (s *Store) RPop(key string) ([]byte, bool, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.liveEntry(key, time.Now())
	if !ok {
		return nil, false, nil
	}
	list, isList := e.value.([][]byte)
	if !isList {
		return nil, false, ErrWrongType
	}
	if len(list) == 0 {
		return nil, false, nil
	}
	v := list[len(list)-1]
	list = list[:len(list)-1]
	if len(list) == 0 {
		delete(sh.data, key)
	} else {
		e.value = list
	}
	return v, true, nil
}

// LLen devuelve la longitud de la lista (0 si no existe).
func (s *Store) LLen(key string) (int, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	list, ok, err := sh.listAt(key, time.Now())
	if err != nil || !ok {
		return 0, err
	}
	return len(list), nil
}

// LRange devuelve el sub-rango [start, stop] con índices estilo Redis
// (negativos cuentan desde el final). Devuelve copia, nunca el slice interno.
func (s *Store) LRange(key string, start, stop int) ([][]byte, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	list, ok, err := sh.listAt(key, time.Now())
	if err != nil {
		return nil, err
	}
	if !ok {
		return [][]byte{}, nil
	}
	n := len(list)
	if start < 0 {
		start += n
	}
	if stop < 0 {
		stop += n
	}
	if start < 0 {
		start = 0
	}
	if stop >= n {
		stop = n - 1
	}
	if start > stop || start >= n {
		return [][]byte{}, nil
	}
	out := make([][]byte, stop-start+1)
	copy(out, list[start:stop+1])
	return out, nil
}
```

- [ ] **Step 4: Ejecutar y verificar que pasa** — `go test ./internal/store/ -run TestList -race -v` → PASS, sin races.

- [ ] **Step 5: Añadir el helper `bulkArray` en `internal/command/handlers.go`**

APPEND al final de `internal/command/handlers.go`:
```go
// bulkArray convierte una lista de valores en un ArrayReply de bulks.
func bulkArray(items [][]byte) protocol.Reply {
	elems := make([]protocol.Reply, len(items))
	for i, it := range items {
		elems[i] = protocol.BulkReply{Value: it}
	}
	return protocol.ArrayReply{Elems: elems}
}
```

- [ ] **Step 6: Crear `internal/command/handlers_list.go`:**
```go
package command

import (
	"strconv"

	"llavero/internal/protocol"
	"llavero/internal/store"
)

func cmdLPush(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) < 2 {
		return protocol.ErrorReply{Msg: "ERR LPUSH requiere clave y al menos un valor"}
	}
	n, err := s.LPush(string(args[0]), args[1:]...)
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	return protocol.IntReply{N: int64(n)}
}

func cmdRPush(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) < 2 {
		return protocol.ErrorReply{Msg: "ERR RPUSH requiere clave y al menos un valor"}
	}
	n, err := s.RPush(string(args[0]), args[1:]...)
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	return protocol.IntReply{N: int64(n)}
}

func cmdLPop(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR LPOP requiere 1 argumento"}
	}
	v, ok, err := s.LPop(string(args[0]))
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	if !ok {
		return protocol.BulkReply{Null: true}
	}
	return protocol.BulkReply{Value: v}
}

func cmdRPop(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR RPOP requiere 1 argumento"}
	}
	v, ok, err := s.RPop(string(args[0]))
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	if !ok {
		return protocol.BulkReply{Null: true}
	}
	return protocol.BulkReply{Value: v}
}

func cmdLLen(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR LLEN requiere 1 argumento"}
	}
	n, err := s.LLen(string(args[0]))
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	return protocol.IntReply{N: int64(n)}
}

func cmdLRange(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 3 {
		return protocol.ErrorReply{Msg: "ERR LRANGE requiere 3 argumentos"}
	}
	start, err1 := strconv.Atoi(string(args[1]))
	stop, err2 := strconv.Atoi(string(args[2]))
	if err1 != nil || err2 != nil {
		return protocol.ErrorReply{Msg: "ERR los índices deben ser enteros"}
	}
	items, err := s.LRange(string(args[0]), start, stop)
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	return bulkArray(items)
}
```

- [ ] **Step 7: Registrar en `internal/command/command.go`** — tras `d.handlers["PERSIST"] = cmdPersist`, añadir:
```go
	d.handlers["LPUSH"] = cmdLPush
	d.handlers["RPUSH"] = cmdRPush
	d.handlers["LPOP"] = cmdLPop
	d.handlers["RPOP"] = cmdRPop
	d.handlers["LLEN"] = cmdLLen
	d.handlers["LRANGE"] = cmdLRange
```

- [ ] **Step 8: Añadir el test de comandos** — APPEND a `internal/command/command_test.go`:
```go
func TestListCommands(t *testing.T) {
	d := NewDispatcher()
	s := store.New(16)
	if r := dispatch(d, s, "RPUSH", "l", "a", "b"); r != (protocol.IntReply{N: 2}) {
		t.Fatalf("RPUSH → %#v", r)
	}
	r := dispatch(d, s, "LRANGE", "l", "0", "-1")
	arr, ok := r.(protocol.ArrayReply)
	if !ok || len(arr.Elems) != 2 {
		t.Fatalf("LRANGE → %#v", r)
	}
	if b, ok := arr.Elems[0].(protocol.BulkReply); !ok || string(b.Value) != "a" {
		t.Fatalf("LRANGE[0] → %#v", arr.Elems[0])
	}
	// WRONGTYPE: comando de lista sobre un string
	dispatch(d, s, "SET", "str", "v")
	if _, ok := dispatch(d, s, "RPUSH", "str", "x").(protocol.ErrorReply); !ok {
		t.Error("RPUSH sobre string debería dar ErrorReply")
	}
}
```

- [ ] **Step 9: Ejecutar y verificar que pasa** — `go test ./internal/store/ ./internal/command/ -race -v` → PASS.

- [ ] **Step 10: Commit**

```bash
git add internal/store/list.go internal/store/list_test.go internal/command/
git -c user.email="developert@orvixapp.com" -c user.name="cesar" commit -m "feat: estructura lista (LPUSH/RPUSH/LPOP/RPOP/LLEN/LRANGE)"
```

---

### Task 4: Hashes (store + comandos)

**Files:**
- Create: `internal/store/hash.go`
- Create: `internal/store/hash_test.go`
- Create: `internal/command/handlers_hash.go`
- Modify: `internal/command/command.go` (registrar)
- Modify: `internal/command/command_test.go` (test)

- [ ] **Step 1: Añadir los tests de store que fallan** — crear `internal/store/hash_test.go`:
```go
package store

import "testing"

func TestHashSetGetDelLen(t *testing.T) {
	s := New(16)
	if isNew, _ := s.HSet("h", "f1", []byte("v1")); !isNew {
		t.Fatal("HSet de campo nuevo → false")
	}
	if isNew, _ := s.HSet("h", "f1", []byte("v1b")); isNew {
		t.Fatal("HSet de campo existente → true")
	}
	s.HSet("h", "f2", []byte("v2"))
	if v, ok, _ := s.HGet("h", "f1"); !ok || string(v) != "v1b" {
		t.Fatalf("HGet → %q %v", v, ok)
	}
	if ln, _ := s.HLen("h"); ln != 2 {
		t.Fatalf("HLen → %d", ln)
	}
	if n, _ := s.HDel("h", "f1", "nope"); n != 1 {
		t.Fatalf("HDel → %d", n)
	}
	if _, ok, _ := s.HGet("h", "f1"); ok {
		t.Fatal("f1 seguía tras HDel")
	}
}

func TestHashGetAll(t *testing.T) {
	s := New(16)
	s.HSet("h", "a", []byte("1"))
	s.HSet("h", "b", []byte("2"))
	flat, _ := s.HGetAll("h")
	if len(flat) != 4 {
		t.Fatalf("HGetAll len → %d", len(flat))
	}
	m := map[string]string{}
	for i := 0; i < len(flat); i += 2 {
		m[string(flat[i])] = string(flat[i+1])
	}
	if m["a"] != "1" || m["b"] != "2" {
		t.Fatalf("HGetAll → %v", m)
	}
}

func TestHashWrongType(t *testing.T) {
	s := New(16)
	s.Set("str", []byte("v"))
	if _, err := s.HSet("str", "f", []byte("v")); err != ErrWrongType {
		t.Fatalf("HSet sobre string → %v", err)
	}
}
```

- [ ] **Step 2: Ejecutar y verificar que falla** — `go test ./internal/store/ -run TestHash -v` → FALLA (HSet/... undefined).

- [ ] **Step 3: Implementar `internal/store/hash.go`:**
```go
package store

import "time"

// hashAt devuelve el hash vivo de key. ok=false si no existe; ErrWrongType si
// la clave tiene otro tipo. El caller debe tener sh.mu tomado.
func (sh *shard) hashAt(key string, now time.Time) (map[string][]byte, bool, error) {
	e, ok := sh.liveEntry(key, now)
	if !ok {
		return nil, false, nil
	}
	h, isHash := e.value.(map[string][]byte)
	if !isHash {
		return nil, false, ErrWrongType
	}
	return h, true, nil
}

// HSet fija un campo. Devuelve true si el campo no existía antes.
func (s *Store) HSet(key, field string, val []byte) (bool, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.liveEntry(key, time.Now())
	if !ok {
		e = &entry{value: map[string][]byte{}}
		sh.data[key] = e
	}
	h, isHash := e.value.(map[string][]byte)
	if !isHash {
		return false, ErrWrongType
	}
	_, existed := h[field]
	h[field] = val
	return !existed, nil
}

// HGet devuelve el valor de un campo. ok=false si no existe el campo o la clave.
func (s *Store) HGet(key, field string) ([]byte, bool, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	h, ok, err := sh.hashAt(key, time.Now())
	if err != nil || !ok {
		return nil, false, err
	}
	v, ok := h[field]
	return v, ok, nil
}

// HDel borra campos y devuelve cuántos existían. Borra la clave si queda vacía.
func (s *Store) HDel(key string, fields ...string) (int, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.liveEntry(key, time.Now())
	if !ok {
		return 0, nil
	}
	h, isHash := e.value.(map[string][]byte)
	if !isHash {
		return 0, ErrWrongType
	}
	n := 0
	for _, f := range fields {
		if _, exists := h[f]; exists {
			delete(h, f)
			n++
		}
	}
	if len(h) == 0 {
		delete(sh.data, key)
	}
	return n, nil
}

// HGetAll devuelve los campos y valores aplanados (campo,valor,campo,valor,...).
func (s *Store) HGetAll(key string) ([][]byte, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	h, ok, err := sh.hashAt(key, time.Now())
	if err != nil {
		return nil, err
	}
	if !ok {
		return [][]byte{}, nil
	}
	out := make([][]byte, 0, len(h)*2)
	for f, v := range h {
		out = append(out, []byte(f), v)
	}
	return out, nil
}

// HLen devuelve el número de campos (0 si no existe).
func (s *Store) HLen(key string) (int, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	h, ok, err := sh.hashAt(key, time.Now())
	if err != nil || !ok {
		return 0, err
	}
	return len(h), nil
}
```

- [ ] **Step 4: Ejecutar y verificar que pasa** — `go test ./internal/store/ -run TestHash -race -v` → PASS.

- [ ] **Step 5: Crear `internal/command/handlers_hash.go`:**
```go
package command

import (
	"llavero/internal/protocol"
	"llavero/internal/store"
)

func cmdHSet(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 3 {
		return protocol.ErrorReply{Msg: "ERR HSET requiere 3 argumentos"}
	}
	isNew, err := s.HSet(string(args[0]), string(args[1]), args[2])
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	if isNew {
		return protocol.IntReply{N: 1}
	}
	return protocol.IntReply{N: 0}
}

func cmdHGet(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 2 {
		return protocol.ErrorReply{Msg: "ERR HGET requiere 2 argumentos"}
	}
	v, ok, err := s.HGet(string(args[0]), string(args[1]))
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	if !ok {
		return protocol.BulkReply{Null: true}
	}
	return protocol.BulkReply{Value: v}
}

func cmdHDel(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) < 2 {
		return protocol.ErrorReply{Msg: "ERR HDEL requiere clave y al menos un campo"}
	}
	fields := make([]string, len(args)-1)
	for i, a := range args[1:] {
		fields[i] = string(a)
	}
	n, err := s.HDel(string(args[0]), fields...)
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	return protocol.IntReply{N: int64(n)}
}

func cmdHGetAll(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR HGETALL requiere 1 argumento"}
	}
	items, err := s.HGetAll(string(args[0]))
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	return bulkArray(items)
}

func cmdHLen(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR HLEN requiere 1 argumento"}
	}
	n, err := s.HLen(string(args[0]))
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	return protocol.IntReply{N: int64(n)}
}
```

- [ ] **Step 6: Registrar en `internal/command/command.go`** — tras la línea `d.handlers["LRANGE"] = cmdLRange`, añadir:
```go
	d.handlers["HSET"] = cmdHSet
	d.handlers["HGET"] = cmdHGet
	d.handlers["HDEL"] = cmdHDel
	d.handlers["HGETALL"] = cmdHGetAll
	d.handlers["HLEN"] = cmdHLen
```

- [ ] **Step 7: Añadir el test de comandos** — APPEND a `internal/command/command_test.go`:
```go
func TestHashCommands(t *testing.T) {
	d := NewDispatcher()
	s := store.New(16)
	if r := dispatch(d, s, "HSET", "h", "f", "v"); r != (protocol.IntReply{N: 1}) {
		t.Fatalf("HSET → %#v", r)
	}
	if r := dispatch(d, s, "HGET", "h", "f"); func() bool { b, ok := r.(protocol.BulkReply); return !ok || string(b.Value) != "v" }() {
		t.Fatalf("HGET → %#v", r)
	}
	r := dispatch(d, s, "HGETALL", "h")
	if arr, ok := r.(protocol.ArrayReply); !ok || len(arr.Elems) != 2 {
		t.Fatalf("HGETALL → %#v", r)
	}
	// WRONGTYPE
	dispatch(d, s, "SET", "str", "v")
	if _, ok := dispatch(d, s, "HSET", "str", "f", "v").(protocol.ErrorReply); !ok {
		t.Error("HSET sobre string debería dar ErrorReply")
	}
}
```

- [ ] **Step 8: Ejecutar y verificar que pasa** — `go test ./internal/store/ ./internal/command/ -race -v` → PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/store/hash.go internal/store/hash_test.go internal/command/
git -c user.email="developert@orvixapp.com" -c user.name="cesar" commit -m "feat: estructura hash (HSET/HGET/HDEL/HGETALL/HLEN)"
```

---

### Task 5: Sets (store + comandos)

**Files:**
- Create: `internal/store/set.go`
- Create: `internal/store/set_test.go`
- Create: `internal/command/handlers_set.go`
- Modify: `internal/command/command.go` (registrar)
- Modify: `internal/command/command_test.go` (test)

- [ ] **Step 1: Añadir los tests de store que fallan** — crear `internal/store/set_test.go`:
```go
package store

import "testing"

func TestSetAddRemMembers(t *testing.T) {
	s := New(16)
	if n, _ := s.SAdd("s", []byte("a"), []byte("b"), []byte("a")); n != 2 {
		t.Fatalf("SAdd → %d, quería 2", n)
	}
	if ok, _ := s.SIsMember("s", []byte("a")); !ok {
		t.Fatal("SIsMember a → false")
	}
	if ok, _ := s.SIsMember("s", []byte("z")); ok {
		t.Fatal("SIsMember z → true")
	}
	if c, _ := s.SCard("s"); c != 2 {
		t.Fatalf("SCard → %d", c)
	}
	if n, _ := s.SRem("s", []byte("a")); n != 1 {
		t.Fatalf("SRem → %d", n)
	}
	mem, _ := s.SMembers("s")
	if len(mem) != 1 || string(mem[0]) != "b" {
		t.Fatalf("SMembers → %v", mem)
	}
}

func TestSetWrongType(t *testing.T) {
	s := New(16)
	s.Set("str", []byte("v"))
	if _, err := s.SAdd("str", []byte("x")); err != ErrWrongType {
		t.Fatalf("SAdd sobre string → %v", err)
	}
}
```

- [ ] **Step 2: Ejecutar y verificar que falla** — `go test ./internal/store/ -run TestSetAdd -v` → FALLA (SAdd/... undefined).

- [ ] **Step 3: Implementar `internal/store/set.go`:**
```go
package store

import "time"

// setAt devuelve el set vivo de key. ok=false si no existe; ErrWrongType si la
// clave tiene otro tipo. El caller debe tener sh.mu tomado.
func (sh *shard) setAt(key string, now time.Time) (map[string]struct{}, bool, error) {
	e, ok := sh.liveEntry(key, now)
	if !ok {
		return nil, false, nil
	}
	set, isSet := e.value.(map[string]struct{})
	if !isSet {
		return nil, false, ErrWrongType
	}
	return set, true, nil
}

// SAdd añade miembros y devuelve cuántos eran nuevos.
func (s *Store) SAdd(key string, members ...[]byte) (int, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.liveEntry(key, time.Now())
	if !ok {
		e = &entry{value: map[string]struct{}{}}
		sh.data[key] = e
	}
	set, isSet := e.value.(map[string]struct{})
	if !isSet {
		return 0, ErrWrongType
	}
	n := 0
	for _, m := range members {
		if _, exists := set[string(m)]; !exists {
			set[string(m)] = struct{}{}
			n++
		}
	}
	return n, nil
}

// SRem quita miembros y devuelve cuántos existían. Borra la clave si queda vacía.
func (s *Store) SRem(key string, members ...[]byte) (int, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.liveEntry(key, time.Now())
	if !ok {
		return 0, nil
	}
	set, isSet := e.value.(map[string]struct{})
	if !isSet {
		return 0, ErrWrongType
	}
	n := 0
	for _, m := range members {
		if _, exists := set[string(m)]; exists {
			delete(set, string(m))
			n++
		}
	}
	if len(set) == 0 {
		delete(sh.data, key)
	}
	return n, nil
}

// SIsMember indica si un miembro pertenece al set.
func (s *Store) SIsMember(key string, member []byte) (bool, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	set, ok, err := sh.setAt(key, time.Now())
	if err != nil || !ok {
		return false, err
	}
	_, exists := set[string(member)]
	return exists, nil
}

// SMembers devuelve todos los miembros (orden no garantizado).
func (s *Store) SMembers(key string) ([][]byte, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	set, ok, err := sh.setAt(key, time.Now())
	if err != nil {
		return nil, err
	}
	if !ok {
		return [][]byte{}, nil
	}
	out := make([][]byte, 0, len(set))
	for m := range set {
		out = append(out, []byte(m))
	}
	return out, nil
}

// SCard devuelve el número de miembros (0 si no existe).
func (s *Store) SCard(key string) (int, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	set, ok, err := sh.setAt(key, time.Now())
	if err != nil || !ok {
		return 0, err
	}
	return len(set), nil
}
```

- [ ] **Step 4: Ejecutar y verificar que pasa** — `go test ./internal/store/ -run TestSet -race -v` → PASS.

- [ ] **Step 5: Crear `internal/command/handlers_set.go`:**
```go
package command

import (
	"llavero/internal/protocol"
	"llavero/internal/store"
)

func cmdSAdd(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) < 2 {
		return protocol.ErrorReply{Msg: "ERR SADD requiere clave y al menos un miembro"}
	}
	n, err := s.SAdd(string(args[0]), args[1:]...)
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	return protocol.IntReply{N: int64(n)}
}

func cmdSRem(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) < 2 {
		return protocol.ErrorReply{Msg: "ERR SREM requiere clave y al menos un miembro"}
	}
	n, err := s.SRem(string(args[0]), args[1:]...)
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	return protocol.IntReply{N: int64(n)}
}

func cmdSIsMember(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 2 {
		return protocol.ErrorReply{Msg: "ERR SISMEMBER requiere 2 argumentos"}
	}
	ok, err := s.SIsMember(string(args[0]), args[1])
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	if ok {
		return protocol.IntReply{N: 1}
	}
	return protocol.IntReply{N: 0}
}

func cmdSMembers(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR SMEMBERS requiere 1 argumento"}
	}
	items, err := s.SMembers(string(args[0]))
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	return bulkArray(items)
}

func cmdSCard(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR SCARD requiere 1 argumento"}
	}
	n, err := s.SCard(string(args[0]))
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	return protocol.IntReply{N: int64(n)}
}
```

- [ ] **Step 6: Registrar en `internal/command/command.go`** — tras la línea `d.handlers["HLEN"] = cmdHLen`, añadir:
```go
	d.handlers["SADD"] = cmdSAdd
	d.handlers["SREM"] = cmdSRem
	d.handlers["SISMEMBER"] = cmdSIsMember
	d.handlers["SMEMBERS"] = cmdSMembers
	d.handlers["SCARD"] = cmdSCard
```

- [ ] **Step 7: Añadir el test de comandos** — APPEND a `internal/command/command_test.go`:
```go
func TestSetCommands(t *testing.T) {
	d := NewDispatcher()
	s := store.New(16)
	if r := dispatch(d, s, "SADD", "s", "a", "b", "a"); r != (protocol.IntReply{N: 2}) {
		t.Fatalf("SADD → %#v", r)
	}
	if r := dispatch(d, s, "SISMEMBER", "s", "a"); r != (protocol.IntReply{N: 1}) {
		t.Fatalf("SISMEMBER → %#v", r)
	}
	if r := dispatch(d, s, "SCARD", "s"); r != (protocol.IntReply{N: 2}) {
		t.Fatalf("SCARD → %#v", r)
	}
	r := dispatch(d, s, "SMEMBERS", "s")
	if arr, ok := r.(protocol.ArrayReply); !ok || len(arr.Elems) != 2 {
		t.Fatalf("SMEMBERS → %#v", r)
	}
	// WRONGTYPE
	dispatch(d, s, "SET", "str", "v")
	if _, ok := dispatch(d, s, "SADD", "str", "x").(protocol.ErrorReply); !ok {
		t.Error("SADD sobre string debería dar ErrorReply")
	}
}
```

- [ ] **Step 8: Ejecutar y verificar que pasa** — `go test ./internal/store/ ./internal/command/ -race -v` → PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/store/set.go internal/store/set_test.go internal/command/
git -c user.email="developert@orvixapp.com" -c user.name="cesar" commit -m "feat: estructura set (SADD/SREM/SISMEMBER/SMEMBERS/SCARD)"
```

---

### Task 6: Verificación final de la fase

**Files:** ninguno (solo verificación).

- [ ] **Step 1: vet + build + suite completa con -race**

Run: `go vet ./... && go build ./... && go test ./... -race`
Expected: vet sin avisos, build OK, todos los paquetes en verde.

- [ ] **Step 2: Prueba de humo manual de estructuras**

Crear `/tmp/smoke4.go`:
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

// readReply lee una respuesta (status/err/int/bulk/array) y la imprime cruda.
func readReply(r *bufio.Reader) string {
	line, _ := r.ReadString('\n')
	if len(line) == 0 {
		return "(vacío)"
	}
	switch line[0] {
	case '+', '-', ':':
		return line[:len(line)-1]
	case '$':
		if line[1] == '-' {
			return "(nil)"
		}
		body, _ := r.ReadString('\n')
		return "$" + body[:len(body)-1]
	case '*':
		var n int
		fmt.Sscanf(line, "*%d", &n)
		out := fmt.Sprintf("array(%d):", n)
		for i := 0; i < n; i++ {
			out += " " + readReply(r)
		}
		return out
	}
	return line
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
	do := func(label string, parts ...string) {
		send(w, parts...)
		fmt.Printf("%-26s -> %s\n", label, readReply(r))
	}

	do("RPUSH lista a b c", "RPUSH", "lista", "a", "b", "c")
	do("LRANGE lista 0 -1", "LRANGE", "lista", "0", "-1")
	do("LPOP lista", "LPOP", "lista")
	do("HSET h campo valor", "HSET", "h", "campo", "valor")
	do("HGET h campo", "HGET", "h", "campo")
	do("HGETALL h", "HGETALL", "h")
	do("SADD s x y x", "SADD", "s", "x", "y", "x")
	do("SCARD s", "SCARD", "s")
	do("SMEMBERS s", "SMEMBERS", "s")
	do("LPUSH lista 1 (WRONGTYPE? no)", "LPUSH", "lista", "1")
	do("HSET lista f v (WRONGTYPE)", "HSET", "lista", "f", "v")
}
```

Run (matar primero cualquier servidor previo en 6380 con `ss -ltnp | grep 6380`):
```bash
go build -o /tmp/llavero-bin ./cmd/llavero
/tmp/llavero-bin &
SRV=$!
go run /tmp/smoke4.go
kill $SRV 2>/dev/null
rm -f /tmp/smoke4.go /tmp/llavero-bin
```
Expected (aprox.):
```
RPUSH lista a b c          -> :3
LRANGE lista 0 -1          -> array(3): $a $b $c
LPOP lista                 -> $a
HSET h campo valor         -> :1
HGET h campo               -> $valor
HGETALL h                  -> array(2): $campo $valor
SADD s x y x               -> :2
SCARD s                    -> :2
SMEMBERS s                 -> array(2): $x $y   (orden no garantizado)
LPUSH lista 1 (WRONGTYPE? no) -> :3
HSET lista f v (WRONGTYPE) -> -WRONGTYPE Operation against a key holding the wrong kind of value
```

---

## Resultado de la fase

Al terminar la Fase 4, Llavero soporta strings, listas, hashes y sets con
respuestas de array y errores WRONGTYPE, además del TTL ya existente. La Fase 5
(persistencia: snapshot + carga) serializará el estado de los shards a disco.
