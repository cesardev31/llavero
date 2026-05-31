package store

import "testing"

func TestSetAddRemMembers(t *testing.T) {
	s := New(16)
	if n, _ := s.SAdd("s", []byte("a"), []byte("b"), []byte("a")); n != 2 {
		t.Fatalf("SAdd -> %d, quería 2", n)
	}
	if ok, _ := s.SIsMember("s", []byte("a")); !ok {
		t.Fatal("SIsMember a -> false")
	}
	if ok, _ := s.SIsMember("s", []byte("z")); ok {
		t.Fatal("SIsMember z -> true")
	}
	if c, _ := s.SCard("s"); c != 2 {
		t.Fatalf("SCard -> %d", c)
	}
	if n, _ := s.SRem("s", []byte("a")); n != 1 {
		t.Fatalf("SRem -> %d", n)
	}
	mem, _ := s.SMembers("s")
	if len(mem) != 1 || string(mem[0]) != "b" {
		t.Fatalf("SMembers -> %v", mem)
	}
}

func TestSetEmptyDeletesKey(t *testing.T) {
	s := New(1)
	s.SAdd("s", []byte("a"))
	s.SRem("s", []byte("a"))
	if len(s.shards[0].data) != 0 {
		t.Fatal("el set vacío no se borró")
	}
}

func TestSetWrongType(t *testing.T) {
	s := New(16)
	s.Set("str", []byte("v"))
	if _, err := s.SAdd("str", []byte("x")); err != ErrWrongType {
		t.Fatalf("SAdd sobre string -> %v", err)
	}
}
