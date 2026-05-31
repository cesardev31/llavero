package store

import (
	"fmt"
	"time"
)

// ValueType identifica el tipo de valor serializado en un snapshot.
type ValueType byte

const (
	ValueString ValueType = 'S'
	ValueList   ValueType = 'L'
	ValueHash   ValueType = 'H'
	ValueSet    ValueType = 'T'
)

// SnapshotEntry es una copia serializable de una clave viva del store.
type SnapshotEntry struct {
	Key      string
	Type     ValueType
	ExpireAt time.Time
	Value    []byte
	List     [][]byte
	Hash     map[string][]byte
	Set      [][]byte
}

// Snapshot devuelve una copia consistente de las claves vivas. Las claves
// vencidas se borran perezosamente durante el recorrido.
func (s *Store) Snapshot() []SnapshotEntry {
	now := time.Now()
	out := make([]SnapshotEntry, 0)
	for _, sh := range s.shards {
		sh.mu.Lock()
		for key, e := range sh.data {
			if e.expired(now) {
				delete(sh.data, key)
				continue
			}
			entry, ok := snapshotEntry(key, e)
			if ok {
				out = append(out, entry)
			}
		}
		sh.mu.Unlock()
	}
	return out
}

func snapshotEntry(key string, e *entry) (SnapshotEntry, bool) {
	out := SnapshotEntry{Key: key, ExpireAt: e.expireAt}
	switch v := e.value.(type) {
	case []byte:
		out.Type = ValueString
		out.Value = cloneBytes(v)
	case [][]byte:
		out.Type = ValueList
		out.List = cloneByteSlices(v)
	case map[string][]byte:
		out.Type = ValueHash
		out.Hash = cloneHash(v)
	case map[string]struct{}:
		out.Type = ValueSet
		out.Set = cloneSet(v)
	default:
		return SnapshotEntry{}, false
	}
	return out, true
}

// Restore reemplaza el contenido del store con las entradas vivas recibidas.
func (s *Store) Restore(entries []SnapshotEntry) error {
	for _, sh := range s.shards {
		sh.mu.Lock()
		sh.data = make(map[string]*entry)
		sh.mu.Unlock()
	}

	now := time.Now()
	for _, snap := range entries {
		if snap.Key == "" {
			return fmt.Errorf("snapshot con clave vacía")
		}
		if !snap.ExpireAt.IsZero() && snap.ExpireAt.Before(now) {
			continue
		}
		val, err := valueFromSnapshot(snap)
		if err != nil {
			return err
		}
		sh := s.shardFor(snap.Key)
		sh.mu.Lock()
		sh.data[snap.Key] = &entry{value: val, expireAt: snap.ExpireAt}
		sh.mu.Unlock()
	}
	return nil
}

func valueFromSnapshot(snap SnapshotEntry) (any, error) {
	switch snap.Type {
	case ValueString:
		return cloneBytes(snap.Value), nil
	case ValueList:
		return cloneByteSlices(snap.List), nil
	case ValueHash:
		return cloneHash(snap.Hash), nil
	case ValueSet:
		set := make(map[string]struct{}, len(snap.Set))
		for _, member := range snap.Set {
			set[string(member)] = struct{}{}
		}
		return set, nil
	default:
		return nil, fmt.Errorf("tipo de snapshot desconocido: %q", byte(snap.Type))
	}
}

func cloneBytes(in []byte) []byte {
	if in == nil {
		return nil
	}
	out := make([]byte, len(in))
	copy(out, in)
	return out
}

func cloneByteSlices(in [][]byte) [][]byte {
	out := make([][]byte, len(in))
	for i, v := range in {
		out[i] = cloneBytes(v)
	}
	return out
}

func cloneHash(in map[string][]byte) map[string][]byte {
	out := make(map[string][]byte, len(in))
	for k, v := range in {
		out[k] = cloneBytes(v)
	}
	return out
}

func cloneSet(in map[string]struct{}) [][]byte {
	out := make([][]byte, 0, len(in))
	for member := range in {
		out = append(out, []byte(member))
	}
	return out
}
