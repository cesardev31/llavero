package protocol

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestRESPParseCommand(t *testing.T) {
	input := "*2\r\n$3\r\nGET\r\n$3\r\nfoo\r\n"
	cmd, err := RESP{}.Parse(bufio.NewReader(strings.NewReader(input)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cmd.Name != "GET" || len(cmd.Args) != 1 || string(cmd.Args[0]) != "foo" {
		t.Fatalf("cmd = %+v", cmd)
	}
}

func TestRESPParseBinaryValue(t *testing.T) {
	val := "con\r\nsaltos y espacios"
	input := fmt.Sprintf("*3\r\n$3\r\nSET\r\n$1\r\nk\r\n$%d\r\n%s\r\n", len(val), val)
	cmd, err := RESP{}.Parse(bufio.NewReader(strings.NewReader(input)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if string(cmd.Args[1]) != val {
		t.Fatalf("valor = %q, quería %q", cmd.Args[1], val)
	}
}

func TestRESPParseRejectsBad(t *testing.T) {
	for _, in := range []string{"GET foo\r\n", "$3\r\nfoo\r\n", "*x\r\n", "*1\r\n+nobulk\r\n"} {
		if _, err := (RESP{}).Parse(bufio.NewReader(strings.NewReader(in))); err == nil {
			t.Errorf("esperaba error para %q", in)
		}
	}
}

func TestRESPEncode(t *testing.T) {
	cases := []struct {
		reply Reply
		want  string
	}{
		{StatusReply{Msg: "OK"}, "+OK\r\n"},
		{ErrorReply{Msg: "ERR x"}, "-ERR x\r\n"},
		{IntReply{N: 7}, ":7\r\n"},
		{BulkReply{Value: []byte("hi")}, "$2\r\nhi\r\n"},
		{BulkReply{Null: true}, "$-1\r\n"},
		{ArrayReply{Elems: []Reply{BulkReply{Value: []byte("a")}, IntReply{N: 1}}}, "*2\r\n$1\r\na\r\n:1\r\n"},
		{ArrayReply{}, "*0\r\n"},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		if err := (RESP{}).Encode(&buf, c.reply); err != nil {
			t.Fatalf("Encode: %v", err)
		}
		if buf.String() != c.want {
			t.Errorf("Encode(%#v) = %q, quería %q", c.reply, buf.String(), c.want)
		}
	}
}
