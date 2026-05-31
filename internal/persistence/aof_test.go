package persistence

import (
	"os"
	"path/filepath"
	"testing"

	"llavero/internal/protocol"
)

func TestAOFAppendAndReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "appendonly.aof")
	aof, err := OpenAOF(path, FsyncAlways)
	if err != nil {
		t.Fatalf("OpenAOF -> %v", err)
	}
	if err := aof.Append(protocol.Command{Name: "SET", Args: [][]byte{[]byte("k"), []byte("v")}}); err != nil {
		t.Fatalf("Append SET -> %v", err)
	}
	if err := aof.Append(protocol.Command{Name: "DEL", Args: [][]byte{[]byte("k")}}); err != nil {
		t.Fatalf("Append DEL -> %v", err)
	}
	if err := aof.Close(); err != nil {
		t.Fatalf("Close -> %v", err)
	}

	var got []string
	if err := ReplayAOF(path, func(cmd protocol.Command) error {
		got = append(got, cmd.Name)
		return nil
	}); err != nil {
		t.Fatalf("ReplayAOF -> %v", err)
	}
	if len(got) != 2 || got[0] != "SET" || got[1] != "DEL" {
		t.Fatalf("comandos replay = %v", got)
	}
}

func TestReplayAOFMissingFileIsNoop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.aof")
	if err := ReplayAOF(path, func(protocol.Command) error {
		t.Fatal("no debería llamar apply")
		return nil
	}); err != nil {
		t.Fatalf("ReplayAOF missing -> %v", err)
	}
}

func TestReplayAOFToleratesTruncatedTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "appendonly.aof")
	aof, err := OpenAOF(path, FsyncNo)
	if err != nil {
		t.Fatalf("OpenAOF -> %v", err)
	}
	if err := aof.Append(protocol.Command{Name: "SET", Args: [][]byte{[]byte("k"), []byte("v")}}); err != nil {
		t.Fatalf("Append -> %v", err)
	}
	if err := aof.Close(); err != nil {
		t.Fatalf("Close -> %v", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("OpenFile append -> %v", err)
	}
	if _, err := f.WriteString("*2\r\n$3\r\nSET\r\n$1\r\n"); err != nil {
		t.Fatalf("WriteString -> %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close tail -> %v", err)
	}

	var n int
	if err := ReplayAOF(path, func(protocol.Command) error {
		n++
		return nil
	}); err != nil {
		t.Fatalf("ReplayAOF truncado -> %v", err)
	}
	if n != 1 {
		t.Fatalf("replay aplicó %d comandos, quería 1", n)
	}
}

func TestParseFsyncPolicy(t *testing.T) {
	for _, raw := range []string{"always", "everysec", "no", "ALWAYS"} {
		if _, err := ParseFsyncPolicy(raw); err != nil {
			t.Fatalf("ParseFsyncPolicy(%q) -> %v", raw, err)
		}
	}
	if _, err := ParseFsyncPolicy("sometimes"); err == nil {
		t.Fatal("ParseFsyncPolicy aceptó política inválida")
	}
}
