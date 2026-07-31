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

func TestApproxMemoryTracksOverwriteMutationAndDelete(t *testing.T) {
	s := New(16)
	s.Set("key", []byte("long-value"))
	if got, want := s.ApproxMemory(), int64(len("key")+len("long-value")); got != want {
		t.Fatalf("initial memory = %d, want %d", got, want)
	}

	s.Set("key", []byte("x"))
	if got, want := s.ApproxMemory(), int64(len("key")+1); got != want {
		t.Fatalf("overwrite memory = %d, want %d", got, want)
	}

	s.HSet("hash", "field", []byte("value"))
	s.HSet("hash", "field", []byte("v"))
	s.RPush("list", []byte("one"), []byte("two"))
	s.LPop("list")
	s.SAdd("set", []byte("one"), []byte("two"))
	s.SRem("set", []byte("one"))

	want := int64(
		len("key") + len("x") +
			len("hash") + len("field") + len("v") +
			len("list") + len("two") +
			len("set") + len("two"),
	)
	if got := s.ApproxMemory(); got != want {
		t.Fatalf("mutated memory = %d, want %d", got, want)
	}

	s.Del("key")
	s.Flush()
	if got := s.ApproxMemory(); got != 0 {
		t.Fatalf("memory after flush = %d, want 0", got)
	}
}

func TestApproxMemoryAndExpiredCounterDropExpiredKeys(t *testing.T) {
	s := New(16)
	s.Set("dead", []byte("value"))
	s.Expire("dead", -1)

	if got := s.EntryMemory("dead"); got != 0 {
		t.Fatalf("expired entry memory = %d, want 0", got)
	}
	if got := s.ApproxMemory(); got != 0 {
		t.Fatalf("memory after expiry = %d, want 0", got)
	}
	if got := s.ExpiredKeys(); got != 1 {
		t.Fatalf("expired keys = %d, want 1", got)
	}
}
