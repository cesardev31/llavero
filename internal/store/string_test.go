package store

import (
	"testing"
	"time"
)

func TestIncrBy(t *testing.T) {
	s := New(16)
	if n, err := s.IncrBy("c", 1); err != nil || n != 1 {
		t.Fatalf("IncrBy nueva → %d, %v", n, err)
	}
	if n, err := s.IncrBy("c", 5); err != nil || n != 6 {
		t.Fatalf("IncrBy → %d, %v", n, err)
	}
	if n, err := s.IncrBy("c", -2); err != nil || n != 4 {
		t.Fatalf("IncrBy negativo → %d, %v", n, err)
	}
	if v, ok, _ := s.Get("c"); !ok || string(v) != "4" {
		t.Fatalf("Get tras IncrBy → %q %v", v, ok)
	}
}

func TestIncrByErrors(t *testing.T) {
	s := New(16)
	s.Set("texto", []byte("abc"))
	if _, err := s.IncrBy("texto", 1); err != ErrNotInteger {
		t.Fatalf("IncrBy sobre no-entero → %v, quería ErrNotInteger", err)
	}
	s.RPush("lista", []byte("x"))
	if _, err := s.IncrBy("lista", 1); err != ErrWrongType {
		t.Fatalf("IncrBy sobre lista → %v, quería ErrWrongType", err)
	}
}

func TestIncrByPreservesTTL(t *testing.T) {
	s := New(16)
	s.Set("c", []byte("1"))
	s.Expire("c", 100*time.Second)
	if _, err := s.IncrBy("c", 1); err != nil {
		t.Fatalf("IncrBy → %v", err)
	}
	if _, _, hasExpiry := s.TTL("c"); !hasExpiry {
		t.Fatal("IncrBy perdió el TTL")
	}
}

func TestSetNX(t *testing.T) {
	s := New(16)
	if !s.SetNX("k", []byte("v1")) {
		t.Fatal("SetNX en clave nueva → false")
	}
	if s.SetNX("k", []byte("v2")) {
		t.Fatal("SetNX en clave existente → true")
	}
	if v, ok, _ := s.Get("k"); !ok || string(v) != "v1" {
		t.Fatalf("valor tras SetNX → %q %v", v, ok)
	}
}
