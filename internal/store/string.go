package store

import (
	"errors"
	"math"
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
		sh.setEntry(key, e)
	}
	if (delta > 0 && n > math.MaxInt64-delta) || (delta < 0 && n < math.MinInt64-delta) {
		return 0, ErrNotInteger
	}
	n += delta
	before := entryApproxMemory(key, e)
	e.value = []byte(strconv.FormatInt(n, 10))
	sh.refreshEntryMemory(key, before)
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
	sh.setEntry(key, &entry{value: val})
	return true
}
