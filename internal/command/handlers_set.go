package command

import (
	"llavero/internal/protocol"
	"llavero/internal/store"
)

func cmdSAdd(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) < 2 {
		return protocol.ErrorReply{Msg: "ERR SADD requiere clave y al menos un miembro"}
	}
	n, err := s.SAdd(string(args[0]), args[1:]...)
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	return protocol.IntReply{N: int64(n)}
}

func cmdSRem(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) < 2 {
		return protocol.ErrorReply{Msg: "ERR SREM requiere clave y al menos un miembro"}
	}
	n, err := s.SRem(string(args[0]), args[1:]...)
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	return protocol.IntReply{N: int64(n)}
}

func cmdSIsMember(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 2 {
		return protocol.ErrorReply{Msg: "ERR SISMEMBER requiere 2 argumentos"}
	}
	ok, err := s.SIsMember(string(args[0]), args[1])
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	if ok {
		return protocol.IntReply{N: 1}
	}
	return protocol.IntReply{N: 0}
}

func cmdSMembers(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR SMEMBERS requiere 1 argumento"}
	}
	items, err := s.SMembers(string(args[0]))
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	return bulkArray(items)
}

func cmdSCard(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR SCARD requiere 1 argumento"}
	}
	n, err := s.SCard(string(args[0]))
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	return protocol.IntReply{N: int64(n)}
}
