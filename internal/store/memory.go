package store

import "time"

// ApproxMemory devuelve en tiempo constante respecto a la cantidad de claves
// la estimación incremental de bytes de claves y valores vivos. No incluye la
// sobrecarga interna de mapas, slices ni del runtime de Go.
func (s *Store) ApproxMemory() int64 {
	var total int64
	for _, sh := range s.shards {
		sh.mu.RLock()
		total += sh.usedMemory
		sh.mu.RUnlock()
	}
	return total
}

// EntryMemory devuelve la estimación viva de una clave. Las claves expiradas
// se eliminan durante la consulta.
func (s *Store) EntryMemory(key string) int64 {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.liveEntry(key, time.Now())
	if !ok {
		return 0
	}
	return entryApproxMemory(key, e)
}

// ExpiredKeys devuelve cuántas claves fueron eliminadas al detectar su TTL
// vencido, tanto por acceso perezoso como por el ciclo activo.
func (s *Store) ExpiredKeys() uint64 {
	var total uint64
	for _, sh := range s.shards {
		sh.mu.RLock()
		total += sh.expiredKeys
		sh.mu.RUnlock()
	}
	return total
}

func (sh *shard) setEntry(key string, e *entry) {
	if old, ok := sh.data[key]; ok {
		sh.usedMemory -= entryApproxMemory(key, old)
	}
	sh.data[key] = e
	sh.usedMemory += entryApproxMemory(key, e)
}

func (sh *shard) deleteEntry(key string) (*entry, bool) {
	e, ok := sh.data[key]
	if !ok {
		return nil, false
	}
	delete(sh.data, key)
	sh.usedMemory -= entryApproxMemory(key, e)
	return e, true
}

func (sh *shard) refreshEntryMemory(key string, before int64) {
	e, ok := sh.data[key]
	if !ok {
		return
	}
	sh.usedMemory += entryApproxMemory(key, e) - before
}

func entryApproxMemory(key string, e *entry) int64 {
	if e == nil {
		return 0
	}
	total := int64(len(key))
	switch value := e.value.(type) {
	case []byte:
		total += int64(len(value))
	case [][]byte:
		for _, item := range value {
			total += int64(len(item))
		}
	case map[string][]byte:
		for field, item := range value {
			total += int64(len(field) + len(item))
		}
	case map[string]struct{}:
		for member := range value {
			total += int64(len(member))
		}
	}
	return total
}
