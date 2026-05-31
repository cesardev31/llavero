package main

import (
	"bufio"
	"strings"
	"testing"
)

func TestSplitLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []string
	}{
		{name: "simple", line: "SET k v", want: []string{"SET", "k", "v"}},
		{name: "double quotes", line: `SET saludo "hola mundo"`, want: []string{"SET", "saludo", "hola mundo"}},
		{name: "single quotes", line: `PUBLISH news 'hola mundo'`, want: []string{"PUBLISH", "news", "hola mundo"}},
		{name: "escaped quote", line: `SET k "hola \"cesar\""`, want: []string{"SET", "k", `hola "cesar"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := splitLine(tt.line)
			if err != nil {
				t.Fatalf("splitLine devolvió error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, quería %d (%v)", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("arg %d = %q, quería %q (todo: %v)", i, got[i], tt.want[i], got)
				}
			}
		})
	}
}

func TestSplitLineErrors(t *testing.T) {
	for _, line := range []string{`SET k "sin cerrar`, `SET k "escape\`} {
		if _, err := splitLine(line); err == nil {
			t.Fatalf("splitLine(%q) no devolvió error", line)
		}
	}
}

func TestReadReplyArray(t *testing.T) {
	input := "*3\r\n$7\r\nmessage\r\n$4\r\nnews\r\n$4\r\nhola\r\n"
	got, err := readReply(bufio.NewReader(strings.NewReader(input)))
	if err != nil {
		t.Fatalf("readReply devolvió error: %v", err)
	}
	if got.kind != replyArray || len(got.items) != 3 {
		t.Fatalf("respuesta = %#v", got)
	}
	if got.items[0].text != "message" || got.items[1].text != "news" || got.items[2].text != "hola" {
		t.Fatalf("items = %#v", got.items)
	}
}

func TestReadReplyNullBulk(t *testing.T) {
	got, err := readReply(bufio.NewReader(strings.NewReader("$-1\r\n")))
	if err != nil {
		t.Fatalf("readReply devolvió error: %v", err)
	}
	if got.kind != replyBulk || !got.null {
		t.Fatalf("respuesta = %#v", got)
	}
}
