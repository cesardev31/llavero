package store

import (
	"testing"
	"time"
)

func TestSetEx_NoExpiry(t *testing.T) {
	s := New(16)
	s.SetEx("k", []byte("v"), time.Time{})
	v, ok, err := s.Get("k")
	if err != nil || !ok || string(v) != "v" {
		t.Fatalf("Get after SetEx: v=%q ok=%v err=%v", v, ok, err)
	}
	if _, _, hasExpiry := s.TTL("k"); hasExpiry {
		t.Fatal("expected no expiry")
	}
}

func TestSetEx_WithExpiry(t *testing.T) {
	s := New(16)
	s.SetEx("k", []byte("v"), time.Now().Add(time.Hour))
	rem, exists, hasExpiry := s.TTL("k")
	if !exists || !hasExpiry || rem <= 0 {
		t.Fatalf("expected live key with future expiry: rem=%v exists=%v hasExpiry=%v", rem, exists, hasExpiry)
	}
}

func TestSetEx_PastExpiryIsMiss(t *testing.T) {
	s := New(16)
	s.SetEx("k", []byte("v"), time.Now().Add(-time.Minute))
	if _, ok, _ := s.Get("k"); ok {
		t.Fatal("expected expired key to be a miss")
	}
}

func TestSetEx_OverwritesPreviousTTL(t *testing.T) {
	s := New(16)
	s.SetEx("k", []byte("v1"), time.Now().Add(time.Hour))
	// Overwrite without expiry must clear the previous TTL.
	s.SetEx("k", []byte("v2"), time.Time{})
	v, ok, _ := s.Get("k")
	if !ok || string(v) != "v2" {
		t.Fatalf("expected v2, got %q ok=%v", v, ok)
	}
	if _, _, hasExpiry := s.TTL("k"); hasExpiry {
		t.Fatal("expected TTL cleared after overwrite")
	}
}
