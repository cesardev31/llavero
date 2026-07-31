package server

import (
	"reflect"
	"testing"
	"time"

	"llavero/internal/protocol"
)

func TestRedactCommandHidesCacheKeysAndValues(t *testing.T) {
	got := redactCommand(protocol.Command{
		Name: "SET",
		Args: [][]byte{[]byte("auth:user-123"), []byte(`{"email":"private@example.com"}`), []byte("EX"), []byte("60")},
	})
	want := []string{"SET", "<redacted>", "<redacted>", "<redacted>", "<redacted>"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("redactCommand() = %#v, want %#v", got, want)
	}
}

func TestLookupStatsCountsGetAndMGet(t *testing.T) {
	hits, misses := lookupStats("GET", nil, protocol.BulkReply{Value: []byte("value")})
	if hits != 1 || misses != 0 {
		t.Fatalf("GET hit = (%d,%d)", hits, misses)
	}

	hits, misses = lookupStats("MGET", nil, protocol.ArrayReply{Elems: []protocol.Reply{
		protocol.BulkReply{Value: []byte("value")},
		protocol.BulkReply{Null: true},
	}})
	if hits != 1 || misses != 1 {
		t.Fatalf("MGET stats = (%d,%d)", hits, misses)
	}
}

func TestShouldLogCommandHonorsMode(t *testing.T) {
	s := &Server{commandLog: commandLogErrors, slowLogThreshold: 10 * time.Millisecond}
	if !s.shouldLogCommand(true, time.Microsecond) || s.shouldLogCommand(false, time.Second) {
		t.Fatal("errors mode did not restrict logs to errors")
	}

	s.commandLog = commandLogSlow
	if s.shouldLogCommand(false, time.Millisecond) || !s.shouldLogCommand(false, 10*time.Millisecond) {
		t.Fatal("slow mode did not honor the slowlog threshold")
	}

	s.commandLog = commandLogOff
	if s.shouldLogCommand(true, time.Second) {
		t.Fatal("off mode logged a command")
	}
}
