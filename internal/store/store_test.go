package store

import (
	"fmt"
	"sync"
	"testing"
	"time"
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

func TestSetClearsTTL(t *testing.T) {
	s := New(16)
	s.Set("k", []byte("v"))
	s.Expire("k", 100*time.Second)
	s.Set("k", []byte("v2")) // debe limpiar el TTL
	_, exists, hasExpiry := s.TTL("k")
	if !exists || hasExpiry {
		t.Fatalf("tras Set, exists=%v hasExpiry=%v; quería exists=true hasExpiry=false", exists, hasExpiry)
	}
}

func TestExpireAndTTL(t *testing.T) {
	s := New(16)
	s.Set("k", []byte("v"))
	if !s.Expire("k", 100*time.Second) {
		t.Fatal("Expire de clave existente devolvió false")
	}
	rem, exists, hasExpiry := s.TTL("k")
	if !exists || !hasExpiry {
		t.Fatalf("exists=%v hasExpiry=%v", exists, hasExpiry)
	}
	if rem <= 0 || rem > 100*time.Second {
		t.Fatalf("restante fuera de rango: %v", rem)
	}
	if s.Expire("nope", time.Second) {
		t.Fatal("Expire de clave inexistente devolvió true")
	}
}

func TestLazyExpireOnGet(t *testing.T) {
	s := New(16)
	s.Set("k", []byte("v"))
	s.Expire("k", -time.Second) // ya vencida
	if _, ok := s.Get("k"); ok {
		t.Fatal("Get devolvió una clave vencida")
	}
}

func TestPersist(t *testing.T) {
	s := New(16)
	s.Set("k", []byte("v"))
	s.Expire("k", 100*time.Second)
	if !s.Persist("k") {
		t.Fatal("Persist devolvió false con TTL presente")
	}
	if _, _, hasExpiry := s.TTL("k"); hasExpiry {
		t.Fatal("seguía con expiración tras Persist")
	}
	if s.Persist("k") {
		t.Fatal("Persist devolvió true sin TTL que quitar")
	}
}

func TestActiveExpireCycleRemovesExpired(t *testing.T) {
	s := New(1) // un solo shard para un test determinista
	for i := 0; i < 50; i++ {
		k := fmt.Sprintf("exp-%d", i)
		s.Set(k, []byte("v"))
		s.Expire(k, -time.Second) // todas vencidas
	}
	s.Set("vivo", []byte("v"))
	s.Set("futuro", []byte("v"))
	s.Expire("futuro", time.Hour)

	s.ActiveExpireCycle()

	total := 0
	for _, sh := range s.shards {
		total += len(sh.data)
	}
	if total != 2 {
		t.Fatalf("tras la ronda quedaban %d claves, quería 2 (vivo y futuro)", total)
	}
	if _, ok := s.Get("vivo"); !ok {
		t.Error("se borró una clave sin TTL")
	}
	if _, ok := s.Get("futuro"); !ok {
		t.Error("se borró una clave con TTL futuro")
	}
}

func TestLazyExpireRemovesFromMap(t *testing.T) {
	s := New(1)
	s.Set("k", []byte("v"))
	s.Expire("k", -time.Second) // ya vencida
	if _, ok := s.Get("k"); ok {
		t.Fatal("Get devolvió una clave vencida")
	}
	// el Get debe haberla borrado del shard, no solo ocultado
	if n := len(s.shards[0].data); n != 0 {
		t.Fatalf("la clave vencida seguía en el mapa: %d entradas", n)
	}
}
