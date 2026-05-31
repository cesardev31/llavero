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
		member := string(m)
		if _, exists := set[member]; !exists {
			set[member] = struct{}{}
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
		member := string(m)
		if _, exists := set[member]; exists {
			delete(set, member)
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
