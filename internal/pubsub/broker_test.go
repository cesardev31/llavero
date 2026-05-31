package pubsub

import (
	"sync"
	"testing"
)

// recorder es un Subscriber de prueba que registra lo que recibe.
type recorder struct {
	mu  sync.Mutex
	got []string
}

func (r *recorder) Deliver(channel string, payload []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got = append(r.got, channel+":"+string(payload))
}

func (r *recorder) list() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.got))
	copy(out, r.got)
	return out
}

func TestPublishDeliversToSubscribers(t *testing.T) {
	b := New()
	a := &recorder{}
	c := &recorder{}
	b.Subscribe(a, "news")
	b.Subscribe(c, "news")
	b.Subscribe(c, "otro")

	if n := b.Publish("news", []byte("hola")); n != 2 {
		t.Fatalf("Publish news → %d receptores, quería 2", n)
	}
	if n := b.Publish("otro", []byte("x")); n != 1 {
		t.Fatalf("Publish otro → %d, quería 1", n)
	}
	if n := b.Publish("vacio", []byte("x")); n != 0 {
		t.Fatalf("Publish a canal sin subs → %d, quería 0", n)
	}
	if got := a.list(); len(got) != 1 || got[0] != "news:hola" {
		t.Fatalf("a recibió %v", got)
	}
	if got := c.list(); len(got) != 2 || got[0] != "news:hola" || got[1] != "otro:x" {
		t.Fatalf("c recibió %v", got)
	}
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	b := New()
	a := &recorder{}
	b.Subscribe(a, "news")
	if !b.Unsubscribe(a, "news") {
		t.Fatal("Unsubscribe de canal suscrito → false")
	}
	if b.Unsubscribe(a, "news") {
		t.Fatal("Unsubscribe repetido → true")
	}
	if n := b.Publish("news", []byte("hola")); n != 0 {
		t.Fatalf("Publish tras unsubscribe → %d, quería 0", n)
	}
	if got := a.list(); len(got) != 0 {
		t.Fatalf("a no debería haber recibido nada: %v", got)
	}
}

func TestSubscribeIdempotent(t *testing.T) {
	b := New()
	a := &recorder{}
	if !b.Subscribe(a, "news") {
		t.Fatal("primera Subscribe → false")
	}
	if b.Subscribe(a, "news") {
		t.Fatal("Subscribe repetida → true")
	}
	if n := b.Publish("news", []byte("x")); n != 1 {
		t.Fatalf("Publish → %d, quería 1 (no duplicado)", n)
	}
}
