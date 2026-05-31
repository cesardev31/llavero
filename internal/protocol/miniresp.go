package protocol

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// ErrProtocol indica una petición que no respeta el formato mini-RESP.
var ErrProtocol = errors.New("protocolo malformado")

// MiniRESP implementa Protocol con marco propio terminado en '\n':
//
//	petición:  *N\n  luego N veces  $len\n<bytes>\n
//	respuesta: +status\n | -err\n | :int\n | $len\n<bytes>\n | $-1\n
type MiniRESP struct{}

// Parse lee una orden completa del reader.
func (MiniRESP) Parse(r *bufio.Reader) (Command, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return Command{}, err
	}
	line = strings.TrimRight(line, "\r\n")
	if len(line) == 0 || line[0] != '*' {
		return Command{}, ErrProtocol
	}
	n, err := strconv.Atoi(line[1:])
	if err != nil || n < 1 {
		return Command{}, ErrProtocol
	}
	parts := make([][]byte, n)
	for i := 0; i < n; i++ {
		b, err := readBulk(r)
		if err != nil {
			return Command{}, err
		}
		parts[i] = b
	}
	return Command{Name: strings.ToUpper(string(parts[0])), Args: parts[1:]}, nil
}

// readBulk lee una parte con prefijo de longitud: $len\n<bytes>\n.
func readBulk(r *bufio.Reader) ([]byte, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimRight(line, "\r\n")
	if len(line) == 0 || line[0] != '$' {
		return nil, ErrProtocol
	}
	n, err := strconv.Atoi(line[1:])
	if err != nil || n < 0 {
		return nil, ErrProtocol
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	// consumir el '\n' (y posible '\r') que cierra los bytes
	if _, err := r.ReadString('\n'); err != nil {
		return nil, err
	}
	return buf, nil
}

// Encode serializa una respuesta al writer.
func (MiniRESP) Encode(w io.Writer, reply Reply) error {
	switch r := reply.(type) {
	case StatusReply:
		_, err := fmt.Fprintf(w, "+%s\n", r.Msg)
		return err
	case ErrorReply:
		_, err := fmt.Fprintf(w, "-%s\n", r.Msg)
		return err
	case IntReply:
		_, err := fmt.Fprintf(w, ":%d\n", r.N)
		return err
	case BulkReply:
		if r.Null {
			_, err := io.WriteString(w, "$-1\n")
			return err
		}
		if _, err := fmt.Fprintf(w, "$%d\n", len(r.Value)); err != nil {
			return err
		}
		if _, err := w.Write(r.Value); err != nil {
			return err
		}
		_, err := io.WriteString(w, "\n")
		return err
	default:
		return fmt.Errorf("tipo de respuesta desconocido: %T", reply)
	}
}
