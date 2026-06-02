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
// El caller debe tener tomado sh.mu en modo escritura, porque puede borrar.
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

// SetEx guarda un valor string con una expiración absoluta, de forma atómica,
// limpiando cualquier TTL o tipo previo. Un expireAt cero significa "sin
// expiración".
func (s *Store) SetEx(key string, val []byte, expireAt time.Time) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e := &entry{value: val}
	if !expireAt.IsZero() {
		e.expireAt = expireAt
	}
	sh.data[key] = e
}

// Get devuelve el valor string de una clave. exists=false si no existe;
// error=ErrWrongType si la clave contiene un tipo que no es string.
func (s *Store) Get(key string) (val []byte, exists bool, err error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.liveEntry(key, time.Now())
	if !ok {
		return nil, false, nil
	}
	b, isStr := e.value.([]byte)
	if !isStr {
		return nil, false, ErrWrongType
	}
	return b, true, nil
}

// Exists indica si la clave existe (de cualquier tipo, con expiración perezosa).
func (s *Store) Exists(key string) bool {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	_, ok := sh.liveEntry(key, time.Now())
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
	return s.ExpireAt(key, time.Now().Add(d))
}

// ExpireAt fija una expiración absoluta para una clave existente. Devuelve si
// la clave existía y no estaba vencida en el momento de aplicar el cambio.
func (s *Store) ExpireAt(key string, at time.Time) bool {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.liveEntry(key, time.Now())
	if !ok {
		return false
	}
	e.expireAt = at
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
