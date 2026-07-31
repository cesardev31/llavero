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
		sh.setEntry(key, e)
	}
	h, isHash := e.value.(map[string][]byte)
	if !isHash {
		return false, ErrWrongType
	}
	before := entryApproxMemory(key, e)
	_, existed := h[field]
	h[field] = val
	sh.refreshEntryMemory(key, before)
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
	before := entryApproxMemory(key, e)
	for _, f := range fields {
		if _, exists := h[f]; exists {
			delete(h, f)
			n++
		}
	}
	if len(h) == 0 {
		sh.deleteEntry(key)
	} else {
		sh.refreshEntryMemory(key, before)
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
