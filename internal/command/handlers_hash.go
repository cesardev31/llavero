package command

import (
	"llavero/internal/protocol"
	"llavero/internal/store"
)

func cmdHSet(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 3 {
		return protocol.ErrorReply{Msg: "ERR HSET requiere 3 argumentos"}
	}
	isNew, err := s.HSet(string(args[0]), string(args[1]), args[2])
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	if isNew {
		return protocol.IntReply{N: 1}
	}
	return protocol.IntReply{N: 0}
}

func cmdHGet(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 2 {
		return protocol.ErrorReply{Msg: "ERR HGET requiere 2 argumentos"}
	}
	v, ok, err := s.HGet(string(args[0]), string(args[1]))
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	if !ok {
		return protocol.BulkReply{Null: true}
	}
	return protocol.BulkReply{Value: v}
}

func cmdHDel(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) < 2 {
		return protocol.ErrorReply{Msg: "ERR HDEL requiere clave y al menos un campo"}
	}
	fields := make([]string, len(args)-1)
	for i, a := range args[1:] {
		fields[i] = string(a)
	}
	n, err := s.HDel(string(args[0]), fields...)
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	return protocol.IntReply{N: int64(n)}
}

func cmdHGetAll(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR HGETALL requiere 1 argumento"}
	}
	items, err := s.HGetAll(string(args[0]))
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	return bulkArray(items)
}

func cmdHLen(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR HLEN requiere 1 argumento"}
	}
	n, err := s.HLen(string(args[0]))
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	return protocol.IntReply{N: int64(n)}
}
