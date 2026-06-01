package server

import (
	"crypto/subtle"
	"strings"

	"llavero/internal/protocol"
)

var commandNames = []string{
	"AUTH", "CLIENT", "COMMAND", "DBSIZE", "DECR", "DECRBY", "DEL", "ECHO",
	"EXISTS", "EXPIRE", "FLUSHALL", "GET", "HELLO", "HDEL", "HGET", "HGETALL",
	"HLEN", "HSET", "INFO", "INCR", "INCRBY", "KEYS", "LLEN", "LPOP", "LPUSH",
	"LRANGE", "MGET", "MSET", "PEXPIREAT", "PERSIST", "PING", "PUBLISH", "QUIT",
	"RPUSH", "RPOP", "SADD", "SCARD", "SELECT", "SET", "SETNX", "SISMEMBER",
	"SLOWLOG", "SMEMBERS", "SREM", "STATS", "SUBSCRIBE", "TTL", "TYPE",
	"UNSUBSCRIBE",
}

func (s *Server) cmdEcho(args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR ECHO requiere 1 argumento"}
	}
	return protocol.BulkReply{Value: args[0]}
}

func (s *Server) cmdSelect(args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR SELECT requiere 1 argumento"}
	}
	if string(args[0]) != "0" {
		return protocol.ErrorReply{Msg: "ERR DB index is out of range"}
	}
	return protocol.StatusReply{Msg: "OK"}
}

func (s *Server) cmdQuit(c *client, args [][]byte) protocol.Reply {
	if len(args) != 0 {
		return protocol.ErrorReply{Msg: "ERR QUIT no recibe argumentos"}
	}
	c.closeAfterReply = true
	return protocol.StatusReply{Msg: "OK"}
}

func (s *Server) cmdHello(c *client, args [][]byte) protocol.Reply {
	if len(args) > 0 && string(args[0]) != "2" {
		return protocol.ErrorReply{Msg: "NOPROTO Llavero soporta RESP2"}
	}
	authedByHello := false
	for i := 1; i < len(args); i++ {
		if strings.EqualFold(string(args[i]), "AUTH") {
			if s.authPassword == "" {
				return protocol.ErrorReply{Msg: "ERR AUTH no requerido"}
			}
			if i+2 >= len(args) {
				return protocol.ErrorReply{Msg: "ERR HELLO AUTH requiere usuario y contraseña"}
			}
			if subtle.ConstantTimeCompare(args[i+2], []byte(s.authPassword)) != 1 {
				return protocol.ErrorReply{Msg: "ERR contraseña inválida"}
			}
			c.authed = true
			authedByHello = true
			i += 2
		}
	}
	if s.authPassword != "" && !c.authed && !authedByHello {
		return protocol.ErrorReply{Msg: "NOAUTH Authentication required."}
	}
	return protocol.ArrayReply{Elems: []protocol.Reply{
		protocol.BulkReply{Value: []byte("server")},
		protocol.BulkReply{Value: []byte("llavero")},
		protocol.BulkReply{Value: []byte("proto")},
		protocol.IntReply{N: 2},
		protocol.BulkReply{Value: []byte("id")},
		protocol.IntReply{N: 1},
		protocol.BulkReply{Value: []byte("mode")},
		protocol.BulkReply{Value: []byte("standalone")},
		protocol.BulkReply{Value: []byte("role")},
		protocol.BulkReply{Value: []byte("master")},
		protocol.BulkReply{Value: []byte("modules")},
		protocol.ArrayReply{},
	}}
}

func (s *Server) cmdCommand(args [][]byte) protocol.Reply {
	if len(args) == 0 {
		elems := make([]protocol.Reply, len(commandNames))
		for i, name := range commandNames {
			elems[i] = commandInfo(name)
		}
		return protocol.ArrayReply{Elems: elems}
	}
	switch strings.ToUpper(string(args[0])) {
	case "COUNT":
		if len(args) != 1 {
			return protocol.ErrorReply{Msg: "ERR COMMAND COUNT no recibe argumentos"}
		}
		return protocol.IntReply{N: int64(len(commandNames))}
	case "DOCS":
		return protocol.ArrayReply{}
	case "INFO":
		elems := make([]protocol.Reply, 0, len(args)-1)
		for _, raw := range args[1:] {
			name := strings.ToUpper(string(raw))
			if knownCommand(name) {
				elems = append(elems, commandInfo(name))
			} else {
				elems = append(elems, protocol.ArrayReply{Null: true})
			}
		}
		return protocol.ArrayReply{Elems: elems}
	case "GETKEYS":
		return protocol.ArrayReply{}
	default:
		return protocol.ErrorReply{Msg: "ERR subcomando COMMAND desconocido"}
	}
}

func commandInfo(name string) protocol.Reply {
	return protocol.ArrayReply{Elems: []protocol.Reply{
		protocol.BulkReply{Value: []byte(strings.ToLower(name))},
		protocol.IntReply{N: int64(commandArity(name))},
		protocol.ArrayReply{Elems: commandFlags(name)},
		protocol.IntReply{N: 0},
		protocol.IntReply{N: 0},
		protocol.IntReply{N: 0},
	}}
}

func commandFlags(name string) []protocol.Reply {
	switch name {
	case "GET", "MGET", "EXISTS", "TTL", "TYPE", "KEYS", "DBSIZE", "INFO", "STATS", "SLOWLOG":
		return []protocol.Reply{protocol.BulkReply{Value: []byte("readonly")}}
	default:
		return []protocol.Reply{protocol.BulkReply{Value: []byte("write")}}
	}
}

func commandArity(name string) int {
	switch name {
	case "PING", "UNSUBSCRIBE", "INFO", "COMMAND":
		return -1
	case "MGET", "DEL", "LPUSH", "RPUSH", "HDEL", "SADD", "SREM", "SUBSCRIBE":
		return -2
	case "MSET":
		return -3
	default:
		return 0
	}
}

func knownCommand(name string) bool {
	for _, known := range commandNames {
		if known == name {
			return true
		}
	}
	return false
}

func (s *Server) cmdClient(args [][]byte) protocol.Reply {
	if len(args) < 1 {
		return protocol.ErrorReply{Msg: "ERR CLIENT requiere subcomando"}
	}
	switch strings.ToUpper(string(args[0])) {
	case "ID":
		if len(args) != 1 {
			return protocol.ErrorReply{Msg: "ERR CLIENT ID no recibe argumentos"}
		}
		return protocol.IntReply{N: 1}
	case "SETINFO":
		if len(args) != 3 {
			return protocol.ErrorReply{Msg: "ERR CLIENT SETINFO requiere campo y valor"}
		}
		return protocol.StatusReply{Msg: "OK"}
	case "SETNAME":
		if len(args) != 2 {
			return protocol.ErrorReply{Msg: "ERR CLIENT SETNAME requiere nombre"}
		}
		return protocol.StatusReply{Msg: "OK"}
	case "GETNAME":
		if len(args) != 1 {
			return protocol.ErrorReply{Msg: "ERR CLIENT GETNAME no recibe argumentos"}
		}
		return protocol.BulkReply{Null: true}
	case "INFO":
		if len(args) != 1 {
			return protocol.ErrorReply{Msg: "ERR CLIENT INFO no recibe argumentos"}
		}
		return protocol.BulkReply{Value: []byte("id=1 name= addr= laddr= fd=-1 age=0 idle=0 flags=N db=0\n")}
	case "LIST":
		if len(args) != 1 {
			return protocol.ErrorReply{Msg: "ERR CLIENT LIST no recibe argumentos"}
		}
		return protocol.BulkReply{Value: []byte("id=1 addr= flags=N db=0\n")}
	default:
		return protocol.ErrorReply{Msg: "ERR subcomando CLIENT desconocido"}
	}
}
