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
