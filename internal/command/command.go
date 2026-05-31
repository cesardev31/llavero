// Package command mapea nombres de comando a handlers que operan sobre el store
// y devuelven una respuesta del protocolo.
package command

import (
	"strings"

	"llavero/internal/protocol"
	"llavero/internal/store"
)

// Handler ejecuta un comando sobre el store y devuelve una respuesta.
type Handler func(s *store.Store, args [][]byte) protocol.Reply

// SaveFunc persiste el store. Se inyecta desde la capa de servidor.
type SaveFunc func(s *store.Store) error

// Dispatcher resuelve el handler de cada comando por nombre.
type Dispatcher struct {
	handlers map[string]Handler
}

// NewDispatcher crea un dispatcher con los comandos soportados registrados.
func NewDispatcher() *Dispatcher {
	return NewDispatcherWithSave(nil)
}

// NewDispatcherWithSave crea un dispatcher y registra SAVE si save no es nil.
func NewDispatcherWithSave(save SaveFunc) *Dispatcher {
	d := &Dispatcher{handlers: make(map[string]Handler)}
	d.handlers["PING"] = cmdPing
	d.handlers["GET"] = cmdGet
	d.handlers["SET"] = cmdSet
	d.handlers["DEL"] = cmdDel
	d.handlers["EXISTS"] = cmdExists
	d.handlers["EXPIRE"] = cmdExpire
	d.handlers["PEXPIREAT"] = cmdPExpireAt
	d.handlers["TTL"] = cmdTTL
	d.handlers["PERSIST"] = cmdPersist
	d.handlers["LPUSH"] = cmdLPush
	d.handlers["RPUSH"] = cmdRPush
	d.handlers["LPOP"] = cmdLPop
	d.handlers["RPOP"] = cmdRPop
	d.handlers["LLEN"] = cmdLLen
	d.handlers["LRANGE"] = cmdLRange
	d.handlers["HSET"] = cmdHSet
	d.handlers["HGET"] = cmdHGet
	d.handlers["HDEL"] = cmdHDel
	d.handlers["HGETALL"] = cmdHGetAll
	d.handlers["HLEN"] = cmdHLen
	d.handlers["SADD"] = cmdSAdd
	d.handlers["SREM"] = cmdSRem
	d.handlers["SISMEMBER"] = cmdSIsMember
	d.handlers["SMEMBERS"] = cmdSMembers
	d.handlers["SCARD"] = cmdSCard
	d.handlers["INCR"] = cmdIncr
	d.handlers["DECR"] = cmdDecr
	d.handlers["INCRBY"] = cmdIncrBy
	d.handlers["DECRBY"] = cmdDecrBy
	d.handlers["MSET"] = cmdMSet
	d.handlers["MGET"] = cmdMGet
	d.handlers["SETNX"] = cmdSetNX
	d.handlers["TYPE"] = cmdType
	d.handlers["KEYS"] = cmdKeys
	d.handlers["DBSIZE"] = cmdDBSize
	d.handlers["FLUSHALL"] = cmdFlushAll
	if save != nil {
		d.handlers["SAVE"] = cmdSave(save)
	}
	return d
}

// Dispatch ejecuta el comando o devuelve un error si no existe.
func (d *Dispatcher) Dispatch(s *store.Store, cmd protocol.Command) protocol.Reply {
	h, ok := d.handlers[strings.ToUpper(cmd.Name)]
	if !ok {
		return protocol.ErrorReply{Msg: "ERR comando desconocido: " + cmd.Name}
	}
	return h(s, cmd.Args)
}
