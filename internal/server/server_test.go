package server

import "testing"

func TestServerListensOnEphemeralPort(t *testing.T) {
	s := New("127.0.0.1:0")
	if err := s.Listen(); err != nil {
		t.Fatalf("Listen devolvió error: %v", err)
	}
	defer s.Close()

	if s.Addr() == "" {
		t.Fatal("Addr() está vacío tras Listen()")
	}
}
