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

// Topes para acotar la memoria que una sola petición puede reservar, evitando
// que un cliente provoque un OOM con una longitud declarada enorme. Reflejan
// los límites por defecto de Redis (multibulk y proto-max-bulk-len).
const (
	maxArgs     = 1024 * 1024       // máximo de partes por orden
	maxBulkSize = 512 * 1024 * 1024 // máximo de bytes por valor (512 MiB)
)

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
	if err != nil || n < 1 || n > maxArgs {
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
	if err != nil || n < 0 || n > maxBulkSize {
		return nil, ErrProtocol
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	// validar el terminador tras los bytes: '\n' o '\r\n'
	term, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	if term == '\r' {
		if term, err = r.ReadByte(); err != nil {
			return nil, err
		}
	}
	if term != '\n' {
		return nil, ErrProtocol
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
	case ArrayReply:
		if r.Null {
			_, err := io.WriteString(w, "*-1\n")
			return err
		}
		if _, err := fmt.Fprintf(w, "*%d\n", len(r.Elems)); err != nil {
			return err
		}
		for _, el := range r.Elems {
			if err := (MiniRESP{}).Encode(w, el); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown reply type: %T", reply)
	}
}
