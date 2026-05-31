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
