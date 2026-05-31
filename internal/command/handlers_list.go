package command

import (
	"strconv"

	"llavero/internal/protocol"
	"llavero/internal/store"
)

func cmdLPush(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) < 2 {
		return protocol.ErrorReply{Msg: "ERR LPUSH requiere clave y al menos un valor"}
	}
	n, err := s.LPush(string(args[0]), args[1:]...)
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	return protocol.IntReply{N: int64(n)}
}

func cmdRPush(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) < 2 {
		return protocol.ErrorReply{Msg: "ERR RPUSH requiere clave y al menos un valor"}
	}
	n, err := s.RPush(string(args[0]), args[1:]...)
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	return protocol.IntReply{N: int64(n)}
}

func cmdLPop(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR LPOP requiere 1 argumento"}
	}
	v, ok, err := s.LPop(string(args[0]))
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	if !ok {
		return protocol.BulkReply{Null: true}
	}
	return protocol.BulkReply{Value: v}
}

func cmdRPop(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR RPOP requiere 1 argumento"}
	}
	v, ok, err := s.RPop(string(args[0]))
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	if !ok {
		return protocol.BulkReply{Null: true}
	}
	return protocol.BulkReply{Value: v}
}

func cmdLLen(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR LLEN requiere 1 argumento"}
	}
	n, err := s.LLen(string(args[0]))
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	return protocol.IntReply{N: int64(n)}
}

func cmdLRange(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 3 {
		return protocol.ErrorReply{Msg: "ERR LRANGE requiere 3 argumentos"}
	}
	start, err1 := strconv.Atoi(string(args[1]))
	stop, err2 := strconv.Atoi(string(args[2]))
	if err1 != nil || err2 != nil {
		return protocol.ErrorReply{Msg: "ERR los índices deben ser enteros"}
	}
	items, err := s.LRange(string(args[0]), start, stop)
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	return bulkArray(items)
}
