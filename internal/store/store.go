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
