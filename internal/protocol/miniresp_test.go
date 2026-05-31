package protocol

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestParseValidCommand(t *testing.T) {
	input := "*3\n$3\nSET\n$6\nnombre\n$5\ncesar\n"
	r := bufio.NewReader(strings.NewReader(input))
	cmd, err := MiniRESP{}.Parse(r)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if cmd.Name != "SET" {
		t.Errorf("Name = %q, quería SET", cmd.Name)
	}
	if len(cmd.Args) != 2 || string(cmd.Args[0]) != "nombre" || string(cmd.Args[1]) != "cesar" {
		t.Errorf("Args = %q", cmd.Args)
	}
}

func TestParseValueWithSpacesAndNewline(t *testing.T) {
	val := "hola mundo\ncon salto"
	input := fmt.Sprintf("*3\n$3\nSET\n$1\nk\n$%d\n%s\n", len(val), val)
	r := bufio.NewReader(strings.NewReader(input))
	cmd, err := MiniRESP{}.Parse(r)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if string(cmd.Args[1]) != val {
		t.Errorf("valor = %q, quería %q", cmd.Args[1], val)
	}
}

func TestParseMalformed(t *testing.T) {
	for _, input := range []string{"SET k v\n", "*abc\n", "*2\nfoo\n"} {
		r := bufio.NewReader(strings.NewReader(input))
		if _, err := (MiniRESP{}).Parse(r); err == nil {
			t.Errorf("esperaba error para %q", input)
		}
	}
}

func TestEncodeReplies(t *testing.T) {
	cases := []struct {
		reply Reply
		want  string
	}{
		{StatusReply{Msg: "OK"}, "+OK\n"},
		{ErrorReply{Msg: "ERR x"}, "-ERR x\n"},
		{IntReply{N: 3}, ":3\n"},
		{BulkReply{Value: []byte("hi")}, "$2\nhi\n"},
		{BulkReply{Null: true}, "$-1\n"},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		if err := (MiniRESP{}).Encode(&buf, c.reply); err != nil {
			t.Fatalf("Encode error: %v", err)
		}
		if buf.String() != c.want {
			t.Errorf("Encode(%#v) = %q, quería %q", c.reply, buf.String(), c.want)
		}
	}
}
