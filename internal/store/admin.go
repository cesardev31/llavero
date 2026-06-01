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
	// Adquirir todos los locks primero para un flush atómico.
	for _, sh := range s.shards {
		sh.mu.Lock()
	}
	for _, sh := range s.shards {
		sh.data = make(map[string]*entry)
	}
	for _, sh := range s.shards {
		sh.mu.Unlock()
	}
}
