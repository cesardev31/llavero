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
	if loggable && s.maxMemoryBytes > 0 {
		s.mutationMu.Lock()
		defer s.mutationMu.Unlock()
		if s.memoryLimitExceeded(logCmd) {
			return protocol.ErrorReply{Msg: "OOM command not allowed when used memory > maxmemory"}
		}
		return s.dispatchMutatingLocked(cmd, logCmd)
	}
	if s.aof == nil || !loggable {
		return s.disp.Dispatch(s.store, cmd)
	}

	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	return s.dispatchMutatingLocked(cmd, logCmd)
}

func (s *Server) dispatchMutatingLocked(cmd, logCmd protocol.Command) protocol.Reply {
	reply := s.disp.Dispatch(s.store, cmd)
	if _, ok := reply.(protocol.ErrorReply); ok {
		return reply
	}
	if s.aof != nil {
		if err := s.aof.Append(logCmd); err != nil {
			return protocol.ErrorReply{Msg: "ERR AOF " + err.Error()}
		}
	}
	return reply
}

func (s *Server) memoryLimitExceeded(cmd protocol.Command) bool {
	growth := estimatedGrowth(cmd)
	if growth <= 0 {
		return false
	}
	return s.store.ApproxMemory()+growth > s.maxMemoryBytes
}

func estimatedGrowth(cmd protocol.Command) int64 {
	name := strings.ToUpper(cmd.Name)
	args := cmd.Args
	switch name {
	case "SET", "SETNX":
		if len(args) < 2 {
			return 0
		}
		return int64(len(args[0]) + len(args[1]))
	case "MSET":
		return argsSize(args)
	case "LPUSH", "RPUSH", "SADD":
		return argsSize(args)
	case "HSET":
		return argsSize(args)
	case "INCR", "DECR":
		if len(args) != 1 {
			return 0
		}
		return int64(len(args[0]) + 32)
	case "INCRBY", "DECRBY":
		if len(args) != 2 {
			return 0
		}
		return int64(len(args[0]) + 32)
	default:
		return 0
	}
}

func argsSize(args [][]byte) int64 {
	var n int64
	for _, arg := range args {
		n += int64(len(arg))
	}
	return n
}

func aofCommand(cmd protocol.Command) (protocol.Command, bool) {
	name := strings.ToUpper(cmd.Name)
	args := cmd.Args
	switch name {
	case "SET":
		if len(args) == 2 {
			return normalizedCommand(name, args), true
		}
		// Normalize SET ... EX/PX/EXAT/PXAT into an absolute PXAT form so AOF
		// replay sets the same expiry regardless of when it runs.
		atMS, ok := setExpiryMillis(args[2:])
		if !ok {
			return protocol.Command{}, false
		}
		norm := [][]byte{args[0], args[1], []byte("PXAT"), []byte(strconv.FormatInt(atMS, 10))}
		return protocol.Command{Name: "SET", Args: norm}, true
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

// setExpiryMillis resolves a single SET expiry option (EX/PX/EXAT/PXAT) to an
// absolute unix-millis timestamp. opts must be exactly [option, value].
func setExpiryMillis(opts [][]byte) (int64, bool) {
	if len(opts) != 2 {
		return 0, false
	}
	n, err := strconv.ParseInt(string(opts[1]), 10, 64)
	if err != nil {
		return 0, false
	}
	switch strings.ToUpper(string(opts[0])) {
	case "EX":
		return time.Now().Add(time.Duration(n) * time.Second).UnixMilli(), true
	case "PX":
		return time.Now().Add(time.Duration(n) * time.Millisecond).UnixMilli(), true
	case "EXAT":
		return time.Unix(n, 0).UnixMilli(), true
	case "PXAT":
		return n, true
	default:
		return 0, false
	}
}

func replayAOFCommand(disp dispatchFunc, cmd protocol.Command) error {
	if _, ok := aofCommand(cmd); !ok {
		return fmt.Errorf("AOF contains non-reproducible command: %s", cmd.Name)
	}
	disp(cmd)
	return nil
}

type dispatchFunc func(protocol.Command) protocol.Reply
