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
