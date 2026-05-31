// Package pubsub enruta mensajes publish/subscribe entre canales y suscriptores.
package pubsub

import "sync"

// Subscriber recibe mensajes de los canales a los que está suscrito.
// La implementación de Deliver no debe entrar en pánico ni asumir orden global.
type Subscriber interface {
	Deliver(channel string, payload []byte)
}

// Broker mantiene, por canal, el conjunto de suscriptores.
type Broker struct {
	mu   sync.RWMutex
	subs map[string]map[Subscriber]struct{}
}

// New crea un broker vacío.
func New() *Broker {
	return &Broker{subs: make(map[string]map[Subscriber]struct{})}
}

// Subscribe añade sub al canal. Devuelve true si no estaba ya suscrito.
func (b *Broker) Subscribe(sub Subscriber, channel string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	set := b.subs[channel]
	if set == nil {
		set = make(map[Subscriber]struct{})
		b.subs[channel] = set
	}
	if _, ok := set[sub]; ok {
		return false
	}
	set[sub] = struct{}{}
	return true
}

// Unsubscribe quita sub del canal. Devuelve true si estaba suscrito.
func (b *Broker) Unsubscribe(sub Subscriber, channel string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	set := b.subs[channel]
	if set == nil {
		return false
	}
	if _, ok := set[sub]; !ok {
		return false
	}
	delete(set, sub)
	if len(set) == 0 {
		delete(b.subs, channel)
	}
	return true
}

// Publish entrega payload a los suscriptores del canal y devuelve cuántos eran.
// Toma una instantánea bajo lock y entrega fuera del lock para no bloquear el
// broker mientras escribe a cada suscriptor.
func (b *Broker) Publish(channel string, payload []byte) int {
	b.mu.RLock()
	set := b.subs[channel]
	targets := make([]Subscriber, 0, len(set))
	for sub := range set {
		targets = append(targets, sub)
	}
	b.mu.RUnlock()

	for _, sub := range targets {
		sub.Deliver(channel, payload)
	}
	return len(targets)
}
