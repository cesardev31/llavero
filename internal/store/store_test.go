package store

import (
	"fmt"
	"sync"
	"testing"
)

func TestSetThenGet(t *testing.T) {
	s := New(16)
	s.Set("k", []byte("v"))
	got, ok := s.Get("k")
	if !ok {
		t.Fatal("Get no encontró la clave recién puesta")
	}
	if string(got) != "v" {
		t.Fatalf("Get = %q, quería v", got)
	}
}

func TestGetMissing(t *testing.T) {
	s := New(16)
	if _, ok := s.Get("nope"); ok {
		t.Fatal("Get devolvió ok=true para clave inexistente")
	}
}

func TestDelAndExists(t *testing.T) {
	s := New(16)
	s.Set("k", []byte("v"))
	if !s.Exists("k") {
		t.Fatal("Exists = false para clave existente")
	}
	if !s.Del("k") {
		t.Fatal("Del = false al borrar clave existente")
	}
	if s.Del("k") {
		t.Fatal("Del = true al borrar clave ya borrada")
	}
	if s.Exists("k") {
		t.Fatal("Exists = true tras borrar")
	}
}

func TestNextPow2(t *testing.T) {
	cases := map[int]int{0: 1, 1: 1, 2: 2, 3: 4, 5: 8, 256: 256, 257: 512}
	for in, want := range cases {
		if got := nextPow2(in); got != want {
			t.Errorf("nextPow2(%d) = %d, quería %d", in, got, want)
		}
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := New(256)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("clave-%d", i)
			s.Set(key, []byte("v"))
			if _, ok := s.Get(key); !ok {
				t.Errorf("no encontró %s", key)
			}
		}(i)
	}
	wg.Wait()
}
