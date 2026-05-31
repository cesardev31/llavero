package store

import "testing"

func TestListPushPopLen(t *testing.T) {
	s := New(16)
	if n, err := s.RPush("l", []byte("a"), []byte("b")); err != nil || n != 2 {
		t.Fatalf("RPush -> %d, %v", n, err)
	}
	if n, err := s.LPush("l", []byte("z")); err != nil || n != 3 {
		t.Fatalf("LPush -> %d, %v", n, err)
	}
	if ln, _ := s.LLen("l"); ln != 3 {
		t.Fatalf("LLen -> %d", ln)
	}
	if v, ok, _ := s.LPop("l"); !ok || string(v) != "z" {
		t.Fatalf("LPop -> %q %v", v, ok)
	}
	if v, ok, _ := s.RPop("l"); !ok || string(v) != "b" {
		t.Fatalf("RPop -> %q %v", v, ok)
	}
}

func TestListRange(t *testing.T) {
	s := New(16)
	s.RPush("l", []byte("a"), []byte("b"), []byte("c"), []byte("d"))
	if got, _ := s.LRange("l", 1, 2); len(got) != 2 || string(got[0]) != "b" || string(got[1]) != "c" {
		t.Fatalf("LRange 1 2 -> %v", got)
	}
	if got, _ := s.LRange("l", 0, -1); len(got) != 4 {
		t.Fatalf("LRange 0 -1 -> %v", got)
	}
	if got, _ := s.LRange("l", -2, -1); len(got) != 2 || string(got[0]) != "c" || string(got[1]) != "d" {
		t.Fatalf("LRange -2 -1 -> %v", got)
	}
	if got, _ := s.LRange("vacia", 0, -1); len(got) != 0 {
		t.Fatalf("LRange inexistente -> %v", got)
	}
}

func TestListWrongType(t *testing.T) {
	s := New(16)
	s.Set("str", []byte("v"))
	if _, err := s.RPush("str", []byte("x")); err != ErrWrongType {
		t.Fatalf("RPush sobre string -> %v, quería ErrWrongType", err)
	}
	if _, err := s.LLen("str"); err != ErrWrongType {
		t.Fatalf("LLen sobre string -> %v", err)
	}
}

func TestListEmptyDeletesKey(t *testing.T) {
	s := New(1)
	s.RPush("l", []byte("a"))
	s.LPop("l")
	if len(s.shards[0].data) != 0 {
		t.Fatal("la lista vacía no se borró")
	}
}
