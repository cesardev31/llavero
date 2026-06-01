// Package protocol define cómo se leen comandos y se escriben respuestas por
// el socket. La interfaz Protocol permite cambiar el formato de cable
// (mini-RESP hoy, RESP real en el futuro) sin tocar el resto del servidor.
package protocol

import (
	"bufio"
	"io"
)

// Command es una orden ya parseada: nombre en mayúsculas + argumentos en bytes.
type Command struct {
	Name string
	Args [][]byte
}

// Reply es cualquier respuesta que el protocolo sabe serializar.
type Reply interface {
	isReply()
}

// StatusReply es una respuesta de estado simple (p.ej. OK).
type StatusReply struct{ Msg string }

// ErrorReply es un error legible para el cliente.
type ErrorReply struct{ Msg string }

// IntReply es una respuesta entera.
type IntReply struct{ N int64 }

// BulkReply es un valor binario; Null indica ausencia de valor.
type BulkReply struct {
	Value []byte
	Null  bool
}

// ArrayReply es una respuesta de array (p.ej. LRANGE, SMEMBERS). Sus elementos
// suelen ser BulkReply, pero puede anidar cualquier Reply. Null indica array
// nulo RESP2 (*-1).
type ArrayReply struct {
	Elems []Reply
	Null  bool
}

func (StatusReply) isReply() {}
func (ErrorReply) isReply()  {}
func (IntReply) isReply()    {}
func (BulkReply) isReply()   {}
func (ArrayReply) isReply()  {}

// Protocol abstrae la lectura de comandos y la escritura de respuestas.
type Protocol interface {
	Parse(r *bufio.Reader) (Command, error)
	Encode(w io.Writer, reply Reply) error
}
