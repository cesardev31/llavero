package store

import "testing"

func TestHashSetGetDelLen(t *testing.T) {
	s := New(16)
	if isNew, _ := s.HSet("h", "f1", []byte("v1")); !isNew {
		t.Fatal("HSet de campo nuevo -> false")
	}
	if isNew, _ := s.HSet("h", "f1", []byte("v1b")); isNew {
		t.Fatal("HSet de campo existente -> true")
	}
	s.HSet("h", "f2", []byte("v2"))
	if v, ok, _ := s.HGet("h", "f1"); !ok || string(v) != "v1b" {
		t.Fatalf("HGet -> %q %v", v, ok)
	}
	if ln, _ := s.HLen("h"); ln != 2 {
		t.Fatalf("HLen -> %d", ln)
	}
	if n, _ := s.HDel("h", "f1", "nope"); n != 1 {
		t.Fatalf("HDel -> %d", n)
	}
	if _, ok, _ := s.HGet("h", "f1"); ok {
		t.Fatal("f1 seguía tras HDel")
	}
}

func TestHashGetAll(t *testing.T) {
	s := New(16)
	s.HSet("h", "a", []byte("1"))
	s.HSet("h", "b", []byte("2"))
	flat, _ := s.HGetAll("h")
	if len(flat) != 4 {
		t.Fatalf("HGetAll len -> %d", len(flat))
	}
	m := map[string]string{}
	for i := 0; i < len(flat); i += 2 {
		m[string(flat[i])] = string(flat[i+1])
	}
	if m["a"] != "1" || m["b"] != "2" {
		t.Fatalf("HGetAll -> %v", m)
	}
}

func TestHashEmptyDeletesKey(t *testing.T) {
	s := New(1)
	s.HSet("h", "f", []byte("v"))
	s.HDel("h", "f")
	if len(s.shards[0].data) != 0 {
		t.Fatal("el hash vacío no se borró")
	}
}

func TestHashWrongType(t *testing.T) {
	s := New(16)
	s.Set("str", []byte("v"))
	if _, err := s.HSet("str", "f", []byte("v")); err != ErrWrongType {
		t.Fatalf("HSet sobre string -> %v", err)
	}
}
