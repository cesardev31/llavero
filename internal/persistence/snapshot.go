// Package persistence guarda y carga snapshots binarios del store.
package persistence

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"llavero/internal/store"
)

var snapshotMagic = []byte{'L', 'L', 'A', 'V', 'E', 'R', 'O', 1}

const (
	maxSnapshotEntries = 1024 * 1024
	maxSnapshotField   = 512 * 1024 * 1024
)

// Save escribe un snapshot atómico del store en path.
func Save(path string, s *store.Store) error {
	if path == "" {
		return errors.New("ruta de snapshot vacía")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(tmpName)
		}
	}()

	bw := bufio.NewWriter(tmp)
	if err := Encode(bw, s.Snapshot()); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := bw.Flush(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	keep = true
	return nil
}

// Load carga un snapshot en el store. Si el archivo no existe, no hace nada.
func Load(path string, s *store.Store) error {
	if path == "" {
		return errors.New("ruta de snapshot vacía")
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer f.Close()

	entries, err := Decode(bufio.NewReader(f))
	if err != nil {
		return err
	}
	return s.Restore(entries)
}

// Encode escribe las entradas en el formato binario propio de Llavero.
func Encode(w io.Writer, entries []store.SnapshotEntry) error {
	if _, err := w.Write(snapshotMagic); err != nil {
		return err
	}
	if err := writeU64(w, uint64(len(entries))); err != nil {
		return err
	}
	for _, entry := range entries {
		if err := encodeEntry(w, entry); err != nil {
			return err
		}
	}
	return nil
}

func encodeEntry(w io.Writer, entry store.SnapshotEntry) error {
	if _, err := w.Write([]byte{byte(entry.Type)}); err != nil {
		return err
	}
	expireAt := int64(0)
	if !entry.ExpireAt.IsZero() {
		expireAt = entry.ExpireAt.UnixNano()
	}
	if err := binary.Write(w, binary.LittleEndian, expireAt); err != nil {
		return err
	}
	if err := writeBytes(w, []byte(entry.Key)); err != nil {
		return err
	}

	switch entry.Type {
	case store.ValueString:
		return writeBytes(w, entry.Value)
	case store.ValueList:
		return writeByteList(w, entry.List)
	case store.ValueHash:
		if err := writeU64(w, uint64(len(entry.Hash))); err != nil {
			return err
		}
		for field, value := range entry.Hash {
			if err := writeBytes(w, []byte(field)); err != nil {
				return err
			}
			if err := writeBytes(w, value); err != nil {
				return err
			}
		}
		return nil
	case store.ValueSet:
		return writeByteList(w, entry.Set)
	default:
		return fmt.Errorf("tipo de snapshot desconocido: %q", byte(entry.Type))
	}
}

// Decode lee entradas desde el formato binario propio de Llavero.
func Decode(r io.Reader) ([]store.SnapshotEntry, error) {
	header := make([]byte, len(snapshotMagic))
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}
	if string(header) != string(snapshotMagic) {
		return nil, errors.New("snapshot inválido: cabecera desconocida")
	}
	count, err := readU64(r)
	if err != nil {
		return nil, err
	}
	if count > maxSnapshotEntries {
		return nil, fmt.Errorf("snapshot inválido: demasiadas entradas (%d)", count)
	}
	entries := make([]store.SnapshotEntry, 0, count)
	for i := uint64(0); i < count; i++ {
		entry, err := decodeEntry(r)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func decodeEntry(r io.Reader) (store.SnapshotEntry, error) {
	var typ [1]byte
	if _, err := io.ReadFull(r, typ[:]); err != nil {
		return store.SnapshotEntry{}, err
	}
	var expireAt int64
	if err := binary.Read(r, binary.LittleEndian, &expireAt); err != nil {
		return store.SnapshotEntry{}, err
	}
	key, err := readBytes(r)
	if err != nil {
		return store.SnapshotEntry{}, err
	}
	entry := store.SnapshotEntry{Key: string(key), Type: store.ValueType(typ[0])}
	if expireAt > 0 {
		entry.ExpireAt = time.Unix(0, expireAt)
	}

	switch entry.Type {
	case store.ValueString:
		entry.Value, err = readBytes(r)
	case store.ValueList:
		entry.List, err = readByteList(r)
	case store.ValueHash:
		entry.Hash, err = readHash(r)
	case store.ValueSet:
		entry.Set, err = readByteList(r)
	default:
		err = fmt.Errorf("snapshot inválido: tipo desconocido %q", typ[0])
	}
	return entry, err
}

func writeByteList(w io.Writer, items [][]byte) error {
	if err := writeU64(w, uint64(len(items))); err != nil {
		return err
	}
	for _, item := range items {
		if err := writeBytes(w, item); err != nil {
			return err
		}
	}
	return nil
}

func readByteList(r io.Reader) ([][]byte, error) {
	count, err := readU64(r)
	if err != nil {
		return nil, err
	}
	if count > maxSnapshotEntries {
		return nil, fmt.Errorf("snapshot inválido: demasiados elementos (%d)", count)
	}
	out := make([][]byte, 0, count)
	for i := uint64(0); i < count; i++ {
		item, err := readBytes(r)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func readHash(r io.Reader) (map[string][]byte, error) {
	count, err := readU64(r)
	if err != nil {
		return nil, err
	}
	if count > maxSnapshotEntries {
		return nil, fmt.Errorf("snapshot inválido: demasiados campos (%d)", count)
	}
	out := make(map[string][]byte, count)
	for i := uint64(0); i < count; i++ {
		field, err := readBytes(r)
		if err != nil {
			return nil, err
		}
		value, err := readBytes(r)
		if err != nil {
			return nil, err
		}
		out[string(field)] = value
	}
	return out, nil
}

func writeBytes(w io.Writer, b []byte) error {
	if err := writeU64(w, uint64(len(b))); err != nil {
		return err
	}
	_, err := w.Write(b)
	return err
}

func readBytes(r io.Reader) ([]byte, error) {
	n, err := readU64(r)
	if err != nil {
		return nil, err
	}
	if n > maxSnapshotField {
		return nil, fmt.Errorf("snapshot inválido: campo demasiado grande (%d bytes)", n)
	}
	out := make([]byte, int(n))
	_, err = io.ReadFull(r, out)
	return out, err
}

func writeU64(w io.Writer, n uint64) error {
	return binary.Write(w, binary.LittleEndian, n)
}

func readU64(r io.Reader) (uint64, error) {
	var n uint64
	err := binary.Read(r, binary.LittleEndian, &n)
	return n, err
}
