package store

import (
	"testing"
	"time"
)

func TestSnapshotRestoreAllTypes(t *testing.T) {
	src := New(16)
	src.Set("str", []byte("v"))
	src.Expire("str", time.Hour)
	src.RPush("list", []byte("a"), []byte("b"))
	src.HSet("hash", "f", []byte("hv"))
	src.SAdd("set", []byte("m"))

	dst := New(16)
	if err := dst.Restore(src.Snapshot()); err != nil {
		t.Fatalf("Restore error: %v", err)
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
	if got, ok, err := dst.HGet("hash", "f"); err != nil || !ok || string(got) != "hv" {
		t.Fatalf("HGet hash -> %q %v %v", got, ok, err)
	}
	if ok, err := dst.SIsMember("set", []byte("m")); err != nil || !ok {
		t.Fatalf("SIsMember set -> %v %v", ok, err)
	}
}

func TestSnapshotSkipsExpiredKeys(t *testing.T) {
	s := New(16)
	s.Set("dead", []byte("v"))
	s.Expire("dead", -time.Second)
	if entries := s.Snapshot(); len(entries) != 0 {
		t.Fatalf("Snapshot incluyó claves vencidas: %v", entries)
	}
	if s.Exists("dead") {
		t.Fatal("Snapshot no limpió la clave vencida")
	}
}

func TestRestoreRejectsUnknownType(t *testing.T) {
	s := New(16)
	err := s.Restore([]SnapshotEntry{{Key: "k", Type: ValueType('?')}})
	if err == nil {
		t.Fatal("Restore aceptó un tipo desconocido")
	}
}
