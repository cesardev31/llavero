package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
)

type replyKind int

const (
	replyStatus replyKind = iota
	replyError
	replyInt
	replyBulk
	replyArray
)

type reply struct {
	kind  replyKind
	text  string
	items []reply
	null  bool
}

func main() {
	addr := flag.String("addr", "127.0.0.1:6380", "dirección TCP de Llavero")
	flag.Parse()

	if flag.NArg() > 0 {
		if err := runCommand(*addr, flag.Args()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := runInteractive(*addr, os.Stdin); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runCommand(addr string, args []string) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("conectar a %s: %w", addr, err)
	}
	defer conn.Close()

	if err := writeCommand(conn, args); err != nil {
		return err
	}
	r := bufio.NewReader(conn)
	if err := readAndPrint(r); err != nil {
		return err
	}
	if isSubscribe(args) {
		for {
			if err := readAndPrint(r); err != nil {
				if errors.Is(err, io.EOF) {
					return nil
				}
				return err
			}
		}
	}
	return nil
}

func runInteractive(addr string, in io.Reader) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("conectar a %s: %w", addr, err)
	}
	defer conn.Close()

	server := bufio.NewReader(conn)
	input := bufio.NewScanner(in)
	for {
		fmt.Print("llavero> ")
		if !input.Scan() {
			fmt.Println()
			return input.Err()
		}
		line := strings.TrimSpace(input.Text())
		if line == "" {
			continue
		}
		if strings.EqualFold(line, "exit") || strings.EqualFold(line, "quit") {
			return nil
		}
		args, err := splitLine(line)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			continue
		}
		if len(args) == 0 {
			continue
		}
		if err := writeCommand(conn, args); err != nil {
			return err
		}
		if err := readAndPrint(server); err != nil {
			return err
		}
		if isSubscribe(args) {
			for {
				if err := readAndPrint(server); err != nil {
					if errors.Is(err, io.EOF) {
						return nil
					}
					return err
				}
			}
		}
	}
}

func isSubscribe(args []string) bool {
	return len(args) > 0 && strings.EqualFold(args[0], "SUBSCRIBE")
}

func writeCommand(w io.Writer, args []string) error {
	if len(args) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "*%d\r\n", len(args)); err != nil {
		return err
	}
	for _, arg := range args {
		if _, err := fmt.Fprintf(w, "$%d\r\n%s\r\n", len(arg), arg); err != nil {
			return err
		}
	}
	return nil
}

func readAndPrint(r *bufio.Reader) error {
	rep, err := readReply(r)
	if err != nil {
		return err
	}
	printReply(rep, "")
	return nil
}

func readReply(r *bufio.Reader) (reply, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return reply{}, err
	}
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return reply{}, errors.New("respuesta vacía")
	}
	switch line[0] {
	case '+':
		return reply{kind: replyStatus, text: line[1:]}, nil
	case '-':
		return reply{kind: replyError, text: line[1:]}, nil
	case ':':
		return reply{kind: replyInt, text: line[1:]}, nil
	case '$':
		n, err := strconv.Atoi(line[1:])
		if err != nil {
			return reply{}, fmt.Errorf("bulk inválido: %q", line)
		}
		if n < 0 {
			return reply{kind: replyBulk, null: true}, nil
		}
		buf := make([]byte, n)
		if _, err := io.ReadFull(r, buf); err != nil {
			return reply{}, err
		}
		if _, err := r.Discard(2); err != nil {
			return reply{}, err
		}
		return reply{kind: replyBulk, text: string(buf)}, nil
	case '*':
		n, err := strconv.Atoi(line[1:])
		if err != nil {
			return reply{}, fmt.Errorf("array inválido: %q", line)
		}
		if n < 0 {
			return reply{kind: replyArray, null: true}, nil
		}
		items := make([]reply, n)
		for i := range items {
			item, err := readReply(r)
			if err != nil {
				return reply{}, err
			}
			items[i] = item
		}
		return reply{kind: replyArray, items: items}, nil
	default:
		return reply{}, fmt.Errorf("tipo de respuesta desconocido: %q", line)
	}
}

func printReply(rep reply, prefix string) {
	switch rep.kind {
	case replyStatus:
		fmt.Println(prefix + rep.text)
	case replyError:
		fmt.Println(prefix + "(error) " + rep.text)
	case replyInt:
		fmt.Println(prefix + "(integer) " + rep.text)
	case replyBulk:
		if rep.null {
			fmt.Println(prefix + "(nil)")
			return
		}
		fmt.Println(prefix + rep.text)
	case replyArray:
		if rep.null {
			fmt.Println(prefix + "(nil)")
			return
		}
		for i, item := range rep.items {
			label := fmt.Sprintf("%s%d) ", prefix, i+1)
			printReply(item, label)
		}
	}
}

func splitLine(line string) ([]string, error) {
	var args []string
	var b strings.Builder
	var quote rune
	escaped := false
	inToken := false

	for _, r := range line {
		if escaped {
			b.WriteRune(r)
			escaped = false
			inToken = true
			continue
		}
		if quote != 0 && r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
				continue
			}
			b.WriteRune(r)
			inToken = true
			continue
		}
		switch {
		case r == '"' || r == '\'':
			quote = r
			inToken = true
		case r == ' ' || r == '\t':
			if inToken {
				args = append(args, b.String())
				b.Reset()
				inToken = false
			}
		default:
			b.WriteRune(r)
			inToken = true
		}
	}
	if escaped {
		return nil, errors.New("escape incompleto")
	}
	if quote != 0 {
		return nil, errors.New("comillas sin cerrar")
	}
	if inToken {
		args = append(args, b.String())
	}
	return args, nil
}
