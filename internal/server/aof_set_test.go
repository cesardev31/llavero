package server

import (
	"strconv"
	"testing"
	"time"

	"llavero/internal/protocol"
)

func TestAOFCommand_PlainSetIsLoggable(t *testing.T) {
	cmd := protocol.Command{Name: "SET", Args: [][]byte{[]byte("k"), []byte("v")}}
	out, ok := aofCommand(cmd)
	if !ok {
		t.Fatal("plain SET should be loggable")
	}
	if out.Name != "SET" || len(out.Args) != 2 {
		t.Fatalf("unexpected normalized command: %+v", out)
	}
}

func TestAOFCommand_SetEXNormalizesToPXAT(t *testing.T) {
	cmd := protocol.Command{Name: "SET", Args: [][]byte{
		[]byte("k"), []byte("v"), []byte("EX"), []byte("100"),
	}}
	out, ok := aofCommand(cmd)
	if !ok {
		t.Fatal("SET ... EX should be loggable")
	}
	if out.Name != "SET" || len(out.Args) != 4 {
		t.Fatalf("expected normalized 4-arg SET, got %+v", out)
	}
	if string(out.Args[2]) != "PXAT" {
		t.Fatalf("expected PXAT normalization, got %q", out.Args[2])
	}
	atMS, err := strconv.ParseInt(string(out.Args[3]), 10, 64)
	if err != nil {
		t.Fatalf("PXAT value not an int: %v", err)
	}
	// ~100s in the future (allow generous slack for test timing).
	delta := time.Until(time.UnixMilli(atMS))
	if delta < 90*time.Second || delta > 110*time.Second {
		t.Fatalf("normalized expiry off: %s", delta)
	}
}

func TestAOFCommand_SetBadOptionNotLoggable(t *testing.T) {
	cmd := protocol.Command{Name: "SET", Args: [][]byte{
		[]byte("k"), []byte("v"), []byte("EX"), []byte("abc"),
	}}
	if _, ok := aofCommand(cmd); ok {
		t.Fatal("SET with non-integer EX must not be loggable")
	}
}
