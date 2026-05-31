package command

import (
	"llavero/internal/protocol"
	"llavero/internal/store"
)

func cmdType(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR TYPE requiere 1 argumento"}
	}
	return protocol.StatusReply{Msg: s.Type(string(args[0]))}
}

func cmdKeys(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR KEYS requiere 1 patrón"}
	}
	items, err := s.Keys(string(args[0]))
	if err != nil {
		return protocol.ErrorReply{Msg: "ERR patrón inválido"}
	}
	return bulkArray(items)
}

func cmdDBSize(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 0 {
		return protocol.ErrorReply{Msg: "ERR DBSIZE no acepta argumentos"}
	}
	return protocol.IntReply{N: int64(s.Len())}
}

func cmdFlushAll(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 0 {
		return protocol.ErrorReply{Msg: "ERR FLUSHALL no acepta argumentos"}
	}
	s.Flush()
	return protocol.StatusReply{Msg: "OK"}
}
