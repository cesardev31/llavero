package persistence

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"llavero/internal/store"
)

func TestSaveLoadRoundTripAllTypes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dump.llavero")
	src := store.New(16)
	src.Set("str", []byte("v"))
	src.Expire("str", time.Hour)
	src.RPush("list", []byte("a"), []byte("b"))
	src.HSet("hash", "field", []byte("hv"))
	src.SAdd("set", []byte("m"))

	if err := Save(path, src); err != nil {
		t.Fatalf("Save error: %v", err)
	}
	dst := store.New(16)
	if err := Load(path, dst); err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if got, ok, err := dst.Get("str"); err != nil || !ok || string(got) != "v" {
		t.Fatalf("Get str -> %q %v %v", got, ok, err)
	}
	if _, exists, hasExpiry := dst.TTL("str"); !exists || !hasExpiry {
		t.Fatalf("TTL str -> exists=%v hasExpiry=%v", exists, hasExpiry)
	}
	if got, err := dst.LRange("list", 0, -1); err != nil || len(got) != 2 || string(got[1]) != "b" {
		t.Fatalf("LRange list -> %v %v", got, err)
	}
	if got, ok, err := dst.HGet("hash", "field"); err != nil || !ok || string(got) != "hv" {
		t.Fatalf("HGet hash -> %q %v %v", got, ok, err)
	}
	if ok, err := dst.SIsMember("set", []byte("m")); err != nil || !ok {
		t.Fatalf("SIsMember set -> %v %v", ok, err)
	}
}

func TestLoadMissingFileIsNoop(t *testing.T) {
	s := store.New(16)
	if err := Load(filepath.Join(t.TempDir(), "missing.llavero"), s); err != nil {
		t.Fatalf("Load missing -> %v", err)
	}
}

func TestDecodeRejectsBadMagic(t *testing.T) {
	if _, err := Decode(bytes.NewReader([]byte("not a snapshot"))); err == nil {
		t.Fatal("Decode aceptó cabecera inválida")
	}
}

func TestSaveSkipsExpiredKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dump.llavero")
	src := store.New(16)
	src.Set("dead", []byte("v"))
	src.Expire("dead", -time.Second)
	if err := Save(path, src); err != nil {
		t.Fatalf("Save error: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat error: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("snapshot vacío en disco")
	}
	dst := store.New(16)
	if err := Load(path, dst); err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if dst.Exists("dead") {
		t.Fatal("Load restauró una clave vencida")
	}
}
