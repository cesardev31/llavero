package command

import (
	"testing"

	"llavero/internal/protocol"
	"llavero/internal/store"
)

// dispatch es un helper que convierte argumentos string a [][]byte y despacha.
func dispatch(d *Dispatcher, s *store.Store, name string, args ...string) protocol.Reply {
	bargs := make([][]byte, len(args))
	for i, a := range args {
		bargs[i] = []byte(a)
	}
	return d.Dispatch(s, protocol.Command{Name: name, Args: bargs})
}

func TestSetGetDelExists(t *testing.T) {
	d := NewDispatcher()
	s := store.New(16)

	if r := dispatch(d, s, "SET", "k", "v"); r != (protocol.StatusReply{Msg: "OK"}) {
		t.Fatalf("SET devolvió %#v", r)
	}
	r := dispatch(d, s, "GET", "k")
	if b, ok := r.(protocol.BulkReply); !ok || string(b.Value) != "v" {
		t.Fatalf("GET devolvió %#v", r)
	}
	if r := dispatch(d, s, "EXISTS", "k"); r != (protocol.IntReply{N: 1}) {
		t.Fatalf("EXISTS devolvió %#v", r)
	}
	if r := dispatch(d, s, "DEL", "k"); r != (protocol.IntReply{N: 1}) {
		t.Fatalf("DEL devolvió %#v", r)
	}
	if r := dispatch(d, s, "GET", "k"); func() bool { b, ok := r.(protocol.BulkReply); return !ok || !b.Null }() {
		t.Fatalf("GET tras DEL debería ser bulk nulo, fue %#v", r)
	}
}

func TestPingReply(t *testing.T) {
	d := NewDispatcher()
	s := store.New(16)
	if r := dispatch(d, s, "PING"); r != (protocol.StatusReply{Msg: "PONG"}) {
		t.Fatalf("PING devolvió %#v", r)
	}
}

func TestUnknownAndArity(t *testing.T) {
	d := NewDispatcher()
	s := store.New(16)
	if _, ok := dispatch(d, s, "NOPE").(protocol.ErrorReply); !ok {
		t.Errorf("comando desconocido debería dar ErrorReply")
	}
	if _, ok := dispatch(d, s, "GET").(protocol.ErrorReply); !ok {
		t.Errorf("GET sin args debería dar ErrorReply")
	}
	if _, ok := dispatch(d, s, "SET", "solo-clave").(protocol.ErrorReply); !ok {
		t.Errorf("SET con 1 arg debería dar ErrorReply")
	}
}

func TestExpireTtlPersist(t *testing.T) {
	d := NewDispatcher()
	s := store.New(16)
	dispatch(d, s, "SET", "k", "v")

	// TTL de clave sin expiración → -1
	if r := dispatch(d, s, "TTL", "k"); r != (protocol.IntReply{N: -1}) {
		t.Fatalf("TTL sin expiración → %#v", r)
	}
	// EXPIRE existente → 1
	if r := dispatch(d, s, "EXPIRE", "k", "100"); r != (protocol.IntReply{N: 1}) {
		t.Fatalf("EXPIRE → %#v", r)
	}
	// TTL ahora ~100 (margen por el redondeo)
	r := dispatch(d, s, "TTL", "k")
	ir, ok := r.(protocol.IntReply)
	if !ok || ir.N < 95 || ir.N > 100 {
		t.Fatalf("TTL tras EXPIRE → %#v", r)
	}
	// PERSIST quita el TTL → 1
	if r := dispatch(d, s, "PERSIST", "k"); r != (protocol.IntReply{N: 1}) {
		t.Fatalf("PERSIST → %#v", r)
	}
	if r := dispatch(d, s, "TTL", "k"); r != (protocol.IntReply{N: -1}) {
		t.Fatalf("TTL tras PERSIST → %#v", r)
	}
	// TTL de clave inexistente → -2
	if r := dispatch(d, s, "TTL", "nope"); r != (protocol.IntReply{N: -2}) {
		t.Fatalf("TTL inexistente → %#v", r)
	}
	// EXPIRE de clave inexistente → 0
	if r := dispatch(d, s, "EXPIRE", "nope", "10"); r != (protocol.IntReply{N: 0}) {
		t.Fatalf("EXPIRE inexistente → %#v", r)
	}
	// EXPIRE con segundos no numéricos → error
	if _, ok := dispatch(d, s, "EXPIRE", "k", "abc").(protocol.ErrorReply); !ok {
		t.Errorf("EXPIRE con segundos no numéricos debería dar ErrorReply")
	}
}

func TestExpireNegativeDeletesKey(t *testing.T) {
	d := NewDispatcher()
	s := store.New(16)
	dispatch(d, s, "SET", "k", "v")
	// EXPIRE con segundos negativos: se aplica (:1) y la clave queda vencida
	if r := dispatch(d, s, "EXPIRE", "k", "-1"); r != (protocol.IntReply{N: 1}) {
		t.Fatalf("EXPIRE -1 → %#v", r)
	}
	// al leerla, debe estar ausente (bulk nulo)
	r := dispatch(d, s, "GET", "k")
	if b, ok := r.(protocol.BulkReply); !ok || !b.Null {
		t.Fatalf("GET tras EXPIRE -1 debería ser bulk nulo, fue %#v", r)
	}
}
