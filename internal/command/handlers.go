package command

import (
	"strconv"
	"time"

	"llavero/internal/protocol"
	"llavero/internal/store"
)

func cmdPing(_ *store.Store, args [][]byte) protocol.Reply {
	if len(args) > 0 {
		return protocol.BulkReply{Value: args[0]}
	}
	return protocol.StatusReply{Msg: "PONG"}
}

func cmdGet(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR GET requiere 1 argumento"}
	}
	v, ok := s.Get(string(args[0]))
	if !ok {
		return protocol.BulkReply{Null: true}
	}
	return protocol.BulkReply{Value: v}
}

func cmdSet(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 2 {
		return protocol.ErrorReply{Msg: "ERR SET requiere 2 argumentos"}
	}
	s.Set(string(args[0]), args[1])
	return protocol.StatusReply{Msg: "OK"}
}

func cmdDel(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) < 1 {
		return protocol.ErrorReply{Msg: "ERR DEL requiere al menos 1 argumento"}
	}
	var n int64
	for _, a := range args {
		if s.Del(string(a)) {
			n++
		}
	}
	return protocol.IntReply{N: n}
}

func cmdExists(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR EXISTS requiere 1 argumento"}
	}
	if s.Exists(string(args[0])) {
		return protocol.IntReply{N: 1}
	}
	return protocol.IntReply{N: 0}
}

func cmdExpire(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 2 {
		return protocol.ErrorReply{Msg: "ERR EXPIRE requiere 2 argumentos"}
	}
	secs, err := strconv.Atoi(string(args[1]))
	if err != nil {
		return protocol.ErrorReply{Msg: "ERR el TTL debe ser un entero"}
	}
	if s.Expire(string(args[0]), time.Duration(secs)*time.Second) {
		return protocol.IntReply{N: 1}
	}
	return protocol.IntReply{N: 0}
}

func cmdTTL(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR TTL requiere 1 argumento"}
	}
	rem, exists, hasExpiry := s.TTL(string(args[0]))
	switch {
	case !exists:
		return protocol.IntReply{N: -2}
	case !hasExpiry:
		return protocol.IntReply{N: -1}
	default:
		// redondeo hacia arriba a segundos
		secs := int64((rem + time.Second - 1) / time.Second)
		return protocol.IntReply{N: secs}
	}
}

func cmdPersist(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR PERSIST requiere 1 argumento"}
	}
	if s.Persist(string(args[0])) {
		return protocol.IntReply{N: 1}
	}
	return protocol.IntReply{N: 0}
}
