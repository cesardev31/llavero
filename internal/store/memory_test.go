package store

import "testing"

func TestApproxMemoryCountsLiveKeysAndValues(t *testing.T) {
	s := New(16)
	s.Set("k", []byte("valor"))
	if got := s.ApproxMemory(); got != int64(len("k")+len("valor")) {
		t.Fatalf("ApproxMemory string = %d", got)
	}
	s.RPush("lista", []byte("a"), []byte("bb"))
	if got := s.ApproxMemory(); got < int64(len("k")+len("valor")+len("lista")+len("a")+len("bb")) {
		t.Fatalf("ApproxMemory no contó lista: %d", got)
	}
}
