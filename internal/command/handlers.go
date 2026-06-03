package command

import (
	"strconv"
	"strings"
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
	v, ok, err := s.Get(string(args[0]))
	if err != nil {
		return protocol.ErrorReply{Msg: err.Error()}
	}
	if !ok {
		return protocol.BulkReply{Null: true}
	}
	return protocol.BulkReply{Value: v}
}

func cmdSet(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) < 2 {
		return protocol.ErrorReply{Msg: "ERR SET requiere 2 argumentos"}
	}
	expireAt, ok := parseSetExpiry(args[2:])
	if !ok {
		return protocol.ErrorReply{Msg: "ERR syntax error"}
	}
	s.SetEx(string(args[0]), args[1], expireAt)
	return protocol.StatusReply{Msg: "OK"}
}

// parseSetExpiry reads the optional SET expiry options (EX/PX/EXAT/PXAT) from the
// trailing args. It returns the resolved absolute expiry (zero = none) and false
// if the options are malformed or mutually exclusive options are combined.
func parseSetExpiry(opts [][]byte) (time.Time, bool) {
	var expireAt time.Time
	set := false
	for i := 0; i < len(opts); {
		opt := strings.ToUpper(string(opts[i]))
		switch opt {
		case "EX", "PX", "EXAT", "PXAT":
			if set || i+1 >= len(opts) {
				return time.Time{}, false
			}
			n, err := strconv.ParseInt(string(opts[i+1]), 10, 64)
			if err != nil {
				return time.Time{}, false
			}
			switch opt {
			case "EX":
				expireAt = time.Now().Add(time.Duration(n) * time.Second)
			case "PX":
				expireAt = time.Now().Add(time.Duration(n) * time.Millisecond)
			case "EXAT":
				expireAt = time.Unix(n, 0)
			case "PXAT":
				expireAt = time.UnixMilli(n)
			}
			set = true
			i += 2
		default:
			return time.Time{}, false
		}
	}
	return expireAt, true
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

func cmdPExpireAt(s *store.Store, args [][]byte) protocol.Reply {
	if len(args) != 2 {
		return protocol.ErrorReply{Msg: "ERR PEXPIREAT requiere 2 argumentos"}
	}
	ms, err := strconv.ParseInt(string(args[1]), 10, 64)
	if err != nil {
		return protocol.ErrorReply{Msg: "ERR el timestamp debe ser un entero"}
	}
	if s.ExpireAt(string(args[0]), time.UnixMilli(ms)) {
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

// bulkArray convierte una lista de valores en un ArrayReply de bulks.
func bulkArray(items [][]byte) protocol.Reply {
	elems := make([]protocol.Reply, len(items))
	for i, it := range items {
		elems[i] = protocol.BulkReply{Value: it}
	}
	return protocol.ArrayReply{Elems: elems}
}

func cmdSave(save SaveFunc) Handler {
	return func(s *store.Store, args [][]byte) protocol.Reply {
		if len(args) != 0 {
			return protocol.ErrorReply{Msg: "ERR SAVE no recibe argumentos"}
		}
		if err := save(s); err != nil {
			return protocol.ErrorReply{Msg: "ERR " + err.Error()}
		}
		return protocol.StatusReply{Msg: "OK"}
	}
}
