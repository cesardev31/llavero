package command

import (
	"strconv"

	"llavero/internal/protocol"
	"llavero/internal/store"
)

func incrByReply(s *store.Store, key string, delta int64) protocol.Reply {
	n, err := s.IncrBy(key, delta)
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	return protocol.IntReply{N: n}
}

func cmdIncr(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR INCR requiere 1 argumento"}
	}
	return incrByReply(s, string(args[0]), 1)
}

func cmdDecr(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR DECR requiere 1 argumento"}
	}
	return incrByReply(s, string(args[0]), -1)
}

func cmdIncrBy(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 2 {
		return protocol.ErrorReply{Msg: "ERR INCRBY requiere 2 argumentos"}
	}
	delta, err := strconv.ParseInt(string(args[1]), 10, 64)
	if err != nil {
		return protocol.ErrorReply{Msg: "ERR el incremento debe ser un entero"}
	}
	return incrByReply(s, string(args[0]), delta)
}

func cmdDecrBy(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 2 {
		return protocol.ErrorReply{Msg: "ERR DECRBY requiere 2 argumentos"}
	}
	delta, err := strconv.ParseInt(string(args[1]), 10, 64)
	if err != nil {
		return protocol.ErrorReply{Msg: "ERR el decremento debe ser un entero"}
	}
	return incrByReply(s, string(args[0]), -delta)
}

func cmdMSet(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) < 2 || len(args)%2 != 0 {
		return protocol.ErrorReply{Msg: "ERR MSET requiere pares clave valor"}
	}
	for i := 0; i < len(args); i += 2 {
		s.Set(string(args[i]), args[i+1])
	}
	return protocol.StatusReply{Msg: "OK"}
}

func cmdMGet(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) < 1 {
		return protocol.ErrorReply{Msg: "ERR MGET requiere al menos 1 clave"}
	}
	elems := make([]protocol.Reply, len(args))
	for i, a := range args {
		v, ok, err := s.Get(string(a))
		if err != nil || !ok {
			elems[i] = protocol.BulkReply{Null: true}
		} else {
			elems[i] = protocol.BulkReply{Value: v}
		}
	}
	return protocol.ArrayReply{Elems: elems}
}

func cmdSetNX(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 2 {
		return protocol.ErrorReply{Msg: "ERR SETNX requiere 2 argumentos"}
	}
	if s.SetNX(string(args[0]), args[1]) {
		return protocol.IntReply{N: 1}
	}
	return protocol.IntReply{N: 0}
}
