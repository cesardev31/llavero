package protocol

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// RESP implementa el protocolo RESP2 de Redis (líneas terminadas en \r\n),
// compatible con clientes reales como redis-cli y go-redis.
//
//	petición:  *N\r\n  luego N veces  $len\r\n<bytes>\r\n
//	respuesta: +s\r\n | -e\r\n | :i\r\n | $len\r\n<bytes>\r\n | $-1\r\n |
//	           *N\r\n<elementos> | *-1\r\n
type RESP struct{}

// Parse lee una orden (array de bulk strings) del reader.
func (RESP) Parse(r *bufio.Reader) (Command, error) {
	line, err := readCRLF(r)
	if err != nil {
		return Command{}, err
	}
	if len(line) == 0 || line[0] != '*' {
		return parseInline(line)
	}
	n, err := strconv.Atoi(line[1:])
	if err != nil || n < 1 || n > maxArgs {
		return Command{}, ErrProtocol
	}
	parts := make([][]byte, n)
	for i := 0; i < n; i++ {
		b, err := readBulkCRLF(r)
		if err != nil {
			return Command{}, err
		}
		parts[i] = b
	}
	return Command{Name: strings.ToUpper(string(parts[0])), Args: parts[1:]}, nil
}

func parseInline(line string) (Command, error) {
	if line == "" {
		return Command{}, ErrProtocol
	}
	switch line[0] {
	case '$', '+', '-', ':':
		return Command{}, ErrProtocol
	}
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return Command{}, ErrProtocol
	}
	args := make([][]byte, len(parts)-1)
	for i, part := range parts[1:] {
		args[i] = []byte(part)
	}
	return Command{Name: strings.ToUpper(parts[0]), Args: args}, nil
}

// readCRLF lee una línea y le quita el \r\n (o \n) final.
func readCRLF(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// readBulkCRLF lee un bulk string $len\r\n<bytes>\r\n.
func readBulkCRLF(r *bufio.Reader) ([]byte, error) {
	line, err := readCRLF(r)
	if err != nil {
		return nil, err
	}
	if len(line) == 0 || line[0] != '$' {
		return nil, ErrProtocol
	}
	n, err := strconv.Atoi(line[1:])
	if err != nil || n < 0 || n > maxBulkSize {
		return nil, ErrProtocol
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	// validar el \r\n final del bulk
	var crlf [2]byte
	if _, err := io.ReadFull(r, crlf[:]); err != nil {
		return nil, err
	}
	if crlf[0] != '\r' || crlf[1] != '\n' {
		return nil, ErrProtocol
	}
	return buf, nil
}

// Encode serializa una respuesta en RESP2.
func (RESP) Encode(w io.Writer, reply Reply) error {
	switch r := reply.(type) {
	case StatusReply:
		_, err := fmt.Fprintf(w, "+%s\r\n", r.Msg)
		return err
	case ErrorReply:
		_, err := fmt.Fprintf(w, "-%s\r\n", r.Msg)
		return err
	case IntReply:
		_, err := fmt.Fprintf(w, ":%d\r\n", r.N)
		return err
	case BulkReply:
		if r.Null {
			_, err := io.WriteString(w, "$-1\r\n")
			return err
		}
		if _, err := fmt.Fprintf(w, "$%d\r\n", len(r.Value)); err != nil {
			return err
		}
		if _, err := w.Write(r.Value); err != nil {
			return err
		}
		_, err := io.WriteString(w, "\r\n")
		return err
	case ArrayReply:
		if r.Null {
			_, err := io.WriteString(w, "*-1\r\n")
			return err
		}
		if _, err := fmt.Fprintf(w, "*%d\r\n", len(r.Elems)); err != nil {
			return err
		}
		for _, el := range r.Elems {
			if err := (RESP{}).Encode(w, el); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown reply type: %T", reply)
	}
}
