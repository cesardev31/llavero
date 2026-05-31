package server

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"llavero/internal/protocol"
)

func (s *Server) dispatchWithAOF(cmd protocol.Command) protocol.Reply {
	logCmd, loggable := aofCommand(cmd)
	if s.aof == nil || !loggable {
		return s.disp.Dispatch(s.store, cmd)
	}

	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	reply := s.disp.Dispatch(s.store, cmd)
	if _, ok := reply.(protocol.ErrorReply); ok {
		return reply
	}
	if err := s.aof.Append(logCmd); err != nil {
		return protocol.ErrorReply{Msg: "ERR AOF " + err.Error()}
	}
	return reply
}

func aofCommand(cmd protocol.Command) (protocol.Command, bool) {
	name := strings.ToUpper(cmd.Name)
	args := cmd.Args
	switch name {
	case "SET":
		return normalizedCommand(name, args), len(args) == 2
	case "DEL":
		return normalizedCommand(name, args), len(args) >= 1
	case "EXPIRE":
		if len(args) != 2 {
			return protocol.Command{}, false
		}
		secs, err := strconv.Atoi(string(args[1]))
		if err != nil {
			return protocol.Command{}, false
		}
		at := time.Now().Add(time.Duration(secs) * time.Second).UnixMilli()
		return protocol.Command{
			Name: "PEXPIREAT",
			Args: [][]byte{args[0], []byte(strconv.FormatInt(at, 10))},
		}, true
	case "PEXPIREAT":
		if len(args) != 2 {
			return protocol.Command{}, false
		}
		if _, err := strconv.ParseInt(string(args[1]), 10, 64); err != nil {
			return protocol.Command{}, false
		}
		return normalizedCommand(name, args), true
	case "PERSIST":
		return normalizedCommand(name, args), len(args) == 1
	case "LPUSH", "RPUSH":
		return normalizedCommand(name, args), len(args) >= 2
	case "LPOP", "RPOP":
		return normalizedCommand(name, args), len(args) == 1
	case "HSET":
		return normalizedCommand(name, args), len(args) == 3
	case "HDEL":
		return normalizedCommand(name, args), len(args) >= 2
	case "SADD", "SREM":
		return normalizedCommand(name, args), len(args) >= 2
	case "INCR", "DECR":
		return normalizedCommand(name, args), len(args) == 1
	case "INCRBY", "DECRBY":
		if len(args) != 2 {
			return protocol.Command{}, false
		}
		if _, err := strconv.ParseInt(string(args[1]), 10, 64); err != nil {
			return protocol.Command{}, false
		}
		return normalizedCommand(name, args), true
	case "MSET":
		return normalizedCommand(name, args), len(args) >= 2 && len(args)%2 == 0
	case "SETNX":
		return normalizedCommand(name, args), len(args) == 2
	case "FLUSHALL":
		return normalizedCommand(name, args), len(args) == 0
	default:
		return protocol.Command{}, false
	}
}

func normalizedCommand(name string, args [][]byte) protocol.Command {
	return protocol.Command{Name: name, Args: args}
}

func replayAOFCommand(disp dispatchFunc, cmd protocol.Command) error {
	if _, ok := aofCommand(cmd); !ok {
		return fmt.Errorf("AOF contiene comando no reproducible: %s", cmd.Name)
	}
	disp(cmd)
	return nil
}

type dispatchFunc func(protocol.Command) protocol.Reply
