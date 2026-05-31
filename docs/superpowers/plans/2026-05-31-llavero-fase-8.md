# Llavero Fase 8 — Plan de Implementación (Capa RESP real)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implementar el protocolo RESP2 de Redis para que `redis-cli` y `go-redis` puedan hablar con Llavero.

**Architecture:** Un nuevo tipo `protocol.RESP` implementa la interfaz `Protocol` con líneas terminadas en `\r\n`, reutilizando los mismos tipos `Reply` y los topes/errores ya definidos. El servidor pasa a usar `RESP{}` por defecto. `MiniRESP` permanece como el protocolo propio original (con sus tests).

**Tech Stack:** Go 1.26, librería estándar (`bufio`, `io`, `strconv`, `strings`, `fmt`). Sin dependencias externas.

## Formato RESP2

- Petición (cliente → servidor): `*N\r\n` y luego N veces `$len\r\n<bytes>\r\n`.
- Respuesta: `+estado\r\n` | `-error\r\n` | `:entero\r\n` | `$len\r\n<bytes>\r\n` | `$-1\r\n` (bulk nulo) | `*N\r\n<elementos>` | `*-1\r\n` (array nulo).

Las constantes `maxArgs`, `maxBulkSize` y el error `ErrProtocol` ya existen en `internal/protocol/miniresp.go` (mismo paquete) y se reutilizan.

## Estructura de archivos

- `internal/protocol/resp.go` + `resp_test.go` — tipo `RESP` (Parse/Encode).
- `internal/server/server.go` — usar `protocol.RESP{}` en lugar de `protocol.MiniRESP{}`.
- `internal/server/server_test.go` — helpers `sendCommand`/`writeCmd` emiten `\r\n`.

---

### Task 1: Tipo `protocol.RESP`

**Files:**
- Create: `internal/protocol/resp.go`
- Create: `internal/protocol/resp_test.go`

- [ ] **Step 1: Escribir los tests que fallan** — crear `internal/protocol/resp_test.go`:
```go
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
```

- [ ] **Step 2: Ejecutar y verificar que falla** — `go test ./internal/protocol/ -run TestRESP -v` → FALLA (undefined: RESP).

- [ ] **Step 3: Implementar `internal/protocol/resp.go`:**
```go
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
		return Command{}, ErrProtocol
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
		return fmt.Errorf("tipo de respuesta desconocido: %T", reply)
	}
}
```

- [ ] **Step 4: Ejecutar y verificar que pasa** — `go test ./internal/protocol/ -v` → PASS en todos (MiniRESP y RESP).

- [ ] **Step 5: Commit**

```bash
git add internal/protocol/resp.go internal/protocol/resp_test.go
git -c user.email="developert@orvixapp.com" -c user.name="cesar" commit -m "feat: protocolo RESP2 (compatible con redis-cli/go-redis)"
```

---

### Task 2: El servidor usa RESP por defecto

**Files:**
- Modify: `internal/server/server.go`
- Modify: `internal/server/server_test.go`

- [ ] **Step 1: Cambiar el protocolo del servidor en `internal/server/server.go`**

En `NewWithOptions`, en el literal de `&Server{...}`, cambiar la línea:
```go
		proto:        protocol.MiniRESP{},
```
por:
```go
		proto:        protocol.RESP{},
```

- [ ] **Step 2: Actualizar los helpers de `internal/server/server_test.go` a `\r\n`**

Reemplazar el cuerpo de `sendCommand` que escribe la orden. La parte actual:
```go
	fmt.Fprintf(conn, "*%d\n", len(parts))
	for _, p := range parts {
		fmt.Fprintf(conn, "$%d\n%s\n", len(p), p)
	}
```
por:
```go
	fmt.Fprintf(conn, "*%d\r\n", len(parts))
	for _, p := range parts {
		fmt.Fprintf(conn, "$%d\r\n%s\r\n", len(p), p)
	}
```

Y reemplazar el cuerpo de `writeCmd`. La parte actual:
```go
	fmt.Fprintf(w, "*%d\n", len(parts))
	for _, p := range parts {
		fmt.Fprintf(w, "$%d\n%s\n", len(p), p)
	}
	w.Flush()
```
por:
```go
	fmt.Fprintf(w, "*%d\r\n", len(parts))
	for _, p := range parts {
		fmt.Fprintf(w, "$%d\r\n%s\r\n", len(p), p)
	}
	w.Flush()
```

(Las aserciones de respuesta no cambian: ya hacen `strings.TrimRight(reply, "\r\n")`.)

- [ ] **Step 3: Ejecutar y verificar que pasa (con -race)** — `go test ./internal/server/ -race -v` → PASS en todos. Los tests ahora ejercen la conversación RESP real sobre TCP.

- [ ] **Step 4: Commit**

```bash
git add internal/server/server.go internal/server/server_test.go
git -c user.email="developert@orvixapp.com" -c user.name="cesar" commit -m "feat: el servidor habla RESP por defecto"
```

---

### Task 3: Verificación final + smoke con cliente real

**Files:** ninguno (solo verificación).

- [ ] **Step 1: vet + build + suite completa con -race**

Run: `go vet ./... && go build ./... && go test ./... -race`
Expected: vet sin avisos, build OK, todos los paquetes en verde.

- [ ] **Step 2: Smoke con `redis-cli` si está disponible**

Comprobar si hay `redis-cli`: `command -v redis-cli`.

Si existe: liberar el puerto 6380 (`ss -ltnp | grep :6380`, matar por PID), arrancar el binario y probar comandos reales:
```bash
go build -o /tmp/llavero-bin ./cmd/llavero
/tmp/llavero-bin -snapshot /tmp/llv-dump &      # o vía background del entorno
SRV=$!
sleep 1
redis-cli -p 6380 PING
redis-cli -p 6380 SET saludo "hola mundo"
redis-cli -p 6380 GET saludo
redis-cli -p 6380 RPUSH lista a b c
redis-cli -p 6380 LRANGE lista 0 -1
redis-cli -p 6380 INCR contador
redis-cli -p 6380 TYPE lista
redis-cli -p 6380 DBSIZE
kill $SRV 2>/dev/null; rm -f /tmp/llavero-bin
```
Expected (aprox.):
```
PONG
OK
"hola mundo"
(integer) 3
1) "a"
2) "b"
3) "c"
(integer) 1
list
(integer) 3
```

Si NO existe `redis-cli`: la compatibilidad RESP queda cubierta por los tests
unitarios de `protocol.RESP` y por los tests de servidor sobre TCP (que ahora
usan framing `\r\n` real). Anotarlo en el reporte y no inventar el resultado.

Nota de entorno: este sandbox puede aislar clientes de red lanzados en segundo
plano; si la salida de `redis-cli` no se captura, basta con la suite + los tests
RESP para dar la fase por verificada.

---

## Resultado de la fase

Al terminar la Fase 8, Llavero habla RESP2 y es compatible con `redis-cli` y
`go-redis`, manteniendo `MiniRESP` como protocolo propio alternativo tras la
interfaz `Protocol`. La Fase 9 añadirá Pub/Sub.
