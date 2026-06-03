package command

import (
	"strconv"
	"testing"
	"time"

	"llavero/internal/protocol"
	"llavero/internal/store"
)

func TestSetWithEX(t *testing.T) {
	d := NewDispatcher()
	s := store.New(16)

	if r := dispatch(d, s, "SET", "k", "v", "EX", "100"); r != (protocol.StatusReply{Msg: "OK"}) {
		t.Fatalf("SET ... EX devolvió %#v", r)
	}
	r := dispatch(d, s, "TTL", "k")
	ttl, ok := r.(protocol.IntReply)
	if !ok || ttl.N <= 0 || ttl.N > 100 {
		t.Fatalf("TTL tras SET EX = %#v", r)
	}
}

func TestSetWithPX(t *testing.T) {
	d := NewDispatcher()
	s := store.New(16)
	if r := dispatch(d, s, "SET", "k", "v", "PX", "100000"); r != (protocol.StatusReply{Msg: "OK"}) {
		t.Fatalf("SET ... PX devolvió %#v", r)
	}
	if r := dispatch(d, s, "TTL", "k"); func() bool { v, ok := r.(protocol.IntReply); return !ok || v.N <= 0 }() {
		t.Fatalf("TTL tras SET PX = %#v", r)
	}
}

func TestSetWithEXAT(t *testing.T) {
	d := NewDispatcher()
	s := store.New(16)
	at := strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)
	if r := dispatch(d, s, "SET", "k", "v", "EXAT", at); r != (protocol.StatusReply{Msg: "OK"}) {
		t.Fatalf("SET ... EXAT devolvió %#v", r)
	}
	if r := dispatch(d, s, "TTL", "k"); func() bool { v, ok := r.(protocol.IntReply); return !ok || v.N <= 0 }() {
		t.Fatalf("TTL tras SET EXAT = %#v", r)
	}
}

func TestSetWithPXAT(t *testing.T) {
	d := NewDispatcher()
	s := store.New(16)
	at := strconv.FormatInt(time.Now().Add(time.Hour).UnixMilli(), 10)
	if r := dispatch(d, s, "SET", "k", "v", "PXAT", at); r != (protocol.StatusReply{Msg: "OK"}) {
		t.Fatalf("SET ... PXAT devolvió %#v", r)
	}
	if r := dispatch(d, s, "TTL", "k"); func() bool { v, ok := r.(protocol.IntReply); return !ok || v.N <= 0 }() {
		t.Fatalf("TTL tras SET PXAT = %#v", r)
	}
}

func TestSetPlainStillWorks(t *testing.T) {
	d := NewDispatcher()
	s := store.New(16)
	if r := dispatch(d, s, "SET", "k", "v"); r != (protocol.StatusReply{Msg: "OK"}) {
		t.Fatalf("SET simple devolvió %#v", r)
	}
	if r := dispatch(d, s, "TTL", "k"); r != (protocol.IntReply{N: -1}) {
		t.Fatalf("SET simple no debe fijar TTL: %#v", r)
	}
}

func TestSetExpiryOptionErrors(t *testing.T) {
	d := NewDispatcher()
	s := store.New(16)
	if _, ok := dispatch(d, s, "SET", "k", "v", "EX", "abc").(protocol.ErrorReply); !ok {
		t.Error("EX con valor no entero debe dar ErrorReply")
	}
	if _, ok := dispatch(d, s, "SET", "k", "v", "EX").(protocol.ErrorReply); !ok {
		t.Error("EX sin valor debe dar ErrorReply")
	}
	if _, ok := dispatch(d, s, "SET", "k", "v", "BOGUS").(protocol.ErrorReply); !ok {
		t.Error("opción desconocida debe dar ErrorReply")
	}
	if _, ok := dispatch(d, s, "SET", "k", "v", "EX", "10", "PX", "10").(protocol.ErrorReply); !ok {
		t.Error("EX y PX juntos deben dar ErrorReply")
	}
}
