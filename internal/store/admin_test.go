package store

import (
	"sort"
	"testing"
	"time"
)

func TestType(t *testing.T) {
	s := New(16)
	s.Set("str", []byte("v"))
	s.RPush("lst", []byte("a"))
	s.HSet("hsh", "f", []byte("v"))
	s.SAdd("set", []byte("m"))
	cases := map[string]string{"str": "string", "lst": "list", "hsh": "hash", "set": "set", "nope": "none"}
	for key, want := range cases {
		if got := s.Type(key); got != want {
			t.Errorf("Type(%q) = %q, quería %q", key, got, want)
		}
	}
}

func TestKeysAndLen(t *testing.T) {
	s := New(16)
	s.Set("user:1", []byte("a"))
	s.Set("user:2", []byte("b"))
	s.Set("otro", []byte("c"))
	if n := s.Len(); n != 3 {
		t.Fatalf("Len → %d, quería 3", n)
	}
	got, err := s.Keys("user:*")
	if err != nil {
		t.Fatalf("Keys error: %v", err)
	}
	strs := make([]string, len(got))
	for i, b := range got {
		strs[i] = string(b)
	}
	sort.Strings(strs)
	if len(strs) != 2 || strs[0] != "user:1" || strs[1] != "user:2" {
		t.Fatalf("Keys user:* → %v", strs)
	}
	all, _ := s.Keys("*")
	if len(all) != 3 {
		t.Fatalf("Keys * → %d, quería 3", len(all))
	}
}

func TestKeysSkipsExpired(t *testing.T) {
	s := New(16)
	s.Set("vivo", []byte("a"))
	s.Set("muerto", []byte("b"))
	s.Expire("muerto", -time.Second)
	if n := s.Len(); n != 1 {
		t.Fatalf("Len con una vencida → %d, quería 1", n)
	}
}

func TestFlush(t *testing.T) {
	s := New(16)
	s.Set("a", []byte("1"))
	s.Set("b", []byte("2"))
	s.Flush()
	if n := s.Len(); n != 0 {
		t.Fatalf("Len tras Flush → %d, quería 0", n)
	}
}
