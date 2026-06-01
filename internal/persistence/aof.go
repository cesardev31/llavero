package persistence

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"llavero/internal/protocol"
)

// FsyncPolicy define cuándo se fuerza el AOF a disco.
type FsyncPolicy string

const (
	FsyncAlways   FsyncPolicy = "always"
	FsyncEverysec FsyncPolicy = "everysec"
	FsyncNo       FsyncPolicy = "no"
)

// ParseFsyncPolicy valida una política de fsync para AOF.
func ParseFsyncPolicy(raw string) (FsyncPolicy, error) {
	switch FsyncPolicy(strings.ToLower(raw)) {
	case FsyncAlways:
		return FsyncAlways, nil
	case FsyncEverysec:
		return FsyncEverysec, nil
	case FsyncNo:
		return FsyncNo, nil
	default:
		return "", fmt.Errorf("invalid AOF policy %q (use always, everysec or no)", raw)
	}
}

// AOF escribe comandos RESP en un append-only file.
type AOF struct {
	mu       sync.Mutex
	file     *os.File
	policy   FsyncPolicy
	lastSync time.Time
}

// OpenAOF abre o crea un AOF para append.
func OpenAOF(path string, policy FsyncPolicy) (*AOF, error) {
	if path == "" {
		return nil, errors.New("empty AOF path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &AOF{file: f, policy: policy}, nil
}

// Append añade cmd al AOF. El comando se codifica completo antes de escribirlo
// para reducir el riesgo de registros intercalados o parciales.
func (a *AOF) Append(cmd protocol.Command) error {
	var buf bytes.Buffer
	if err := EncodeCommand(&buf, cmd); err != nil {
		return err
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if _, err := a.file.Write(buf.Bytes()); err != nil {
		return err
	}
	switch a.policy {
	case FsyncAlways:
		if err := a.file.Sync(); err != nil {
			return err
		}
		a.lastSync = time.Now()
	case FsyncEverysec:
		if time.Since(a.lastSync) >= time.Second {
			if err := a.file.Sync(); err != nil {
				return err
			}
			a.lastSync = time.Now()
		}
	case FsyncNo:
	}
	return nil
}

// Close sincroniza y cierra el archivo.
func (a *AOF) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.file.Sync(); err != nil {
		_ = a.file.Close()
		return err
	}
	return a.file.Close()
}

// EncodeCommand escribe un comando en formato RESP2.
func EncodeCommand(w io.Writer, cmd protocol.Command) error {
	parts := make([][]byte, 0, len(cmd.Args)+1)
	parts = append(parts, []byte(strings.ToUpper(cmd.Name)))
	parts = append(parts, cmd.Args...)
	if _, err := fmt.Fprintf(w, "*%d\r\n", len(parts)); err != nil {
		return err
	}
	for _, part := range parts {
		if _, err := fmt.Fprintf(w, "$%d\r\n", len(part)); err != nil {
			return err
		}
		if _, err := w.Write(part); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "\r\n"); err != nil {
			return err
		}
	}
	return nil
}

// ReplayAOF lee path y llama apply por cada comando completo. Si el archivo no
// existe, no hace nada. Un EOF limpio o un último registro truncado se ignoran,
// porque pueden aparecer tras un crash durante append.
func ReplayAOF(path string, apply func(protocol.Command) error) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer f.Close()

	r := bufio.NewReader(f)
	for {
		cmd, err := (protocol.RESP{}).Parse(r)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil
			}
			return err
		}
		if err := apply(cmd); err != nil {
			return err
		}
	}
}
