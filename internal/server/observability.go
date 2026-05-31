package server

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"llavero/internal/protocol"
)

const defaultSlowLogMaxLen = 128

type metrics struct {
	mu                  sync.Mutex
	startedAt           time.Time
	totalCommands       uint64
	totalErrors         uint64
	totalConnections    uint64
	currentConnections  uint64
	rejectedConnections uint64
	perCommand          map[string]uint64
	nextSlowID          int64
	slowLog             []slowEntry
}

type slowEntry struct {
	id       int64
	at       time.Time
	duration time.Duration
	cmd      []string
}

func newMetrics() *metrics {
	return &metrics{startedAt: time.Now(), perCommand: make(map[string]uint64)}
}

func (s *Server) recordConnectionAccepted() {
	s.metrics.mu.Lock()
	defer s.metrics.mu.Unlock()
	s.metrics.totalConnections++
	s.metrics.currentConnections++
}

func (s *Server) recordConnectionRejected() {
	s.metrics.mu.Lock()
	defer s.metrics.mu.Unlock()
	s.metrics.rejectedConnections++
}

func (s *Server) recordConnectionClosed() {
	s.metrics.mu.Lock()
	defer s.metrics.mu.Unlock()
	if s.metrics.currentConnections > 0 {
		s.metrics.currentConnections--
	}
}

func (s *Server) observeCommand(remote string, cmd protocol.Command, reply protocol.Reply, duration time.Duration) {
	name := strings.ToUpper(cmd.Name)
	isErr := isErrorReply(reply)

	s.metrics.mu.Lock()
	s.metrics.totalCommands++
	if isErr {
		s.metrics.totalErrors++
	}
	s.metrics.perCommand[name]++
	if s.slowLogThreshold > 0 && duration >= s.slowLogThreshold && !isSlowLogReset(cmd) {
		s.metrics.nextSlowID++
		s.metrics.slowLog = append([]slowEntry{{
			id:       s.metrics.nextSlowID,
			at:       time.Now(),
			duration: duration,
			cmd:      redactCommand(cmd),
		}}, s.metrics.slowLog...)
		if len(s.metrics.slowLog) > s.slowLogMaxLen {
			s.metrics.slowLog = s.metrics.slowLog[:s.slowLogMaxLen]
		}
	}
	s.metrics.mu.Unlock()

	log.Printf("event=command remote=%q cmd=%q duration_us=%d error=%t", remote, name, duration.Microseconds(), isErr)
}

func isSlowLogReset(cmd protocol.Command) bool {
	return strings.EqualFold(cmd.Name, "SLOWLOG") &&
		len(cmd.Args) == 1 &&
		strings.EqualFold(string(cmd.Args[0]), "RESET")
}

func (s *Server) recordProtocolError(remote string, err error) {
	s.metrics.mu.Lock()
	s.metrics.totalErrors++
	s.metrics.mu.Unlock()
	log.Printf("event=protocol_error remote=%q error=%q", remote, err)
}

func isErrorReply(reply protocol.Reply) bool {
	if reply == nil {
		return false
	}
	_, ok := reply.(protocol.ErrorReply)
	return ok
}

func redactCommand(cmd protocol.Command) []string {
	name := strings.ToUpper(cmd.Name)
	out := make([]string, 0, len(cmd.Args)+1)
	out = append(out, name)
	for i, arg := range cmd.Args {
		if name == "AUTH" && i == 0 {
			out = append(out, "<redacted>")
			continue
		}
		out = append(out, string(arg))
	}
	return out
}

func (s *Server) cmdInfo(args [][]byte) protocol.Reply {
	if len(args) > 1 {
		return protocol.ErrorReply{Msg: "ERR INFO recibe cero o una sección"}
	}
	return protocol.BulkReply{Value: []byte(s.infoString())}
}

func (s *Server) cmdStats(args [][]byte) protocol.Reply {
	if len(args) != 0 {
		return protocol.ErrorReply{Msg: "ERR STATS no recibe argumentos"}
	}
	return protocol.BulkReply{Value: []byte(s.infoString())}
}

func (s *Server) infoString() string {
	s.metrics.mu.Lock()
	startedAt := s.metrics.startedAt
	totalCommands := s.metrics.totalCommands
	totalErrors := s.metrics.totalErrors
	totalConnections := s.metrics.totalConnections
	currentConnections := s.metrics.currentConnections
	rejectedConnections := s.metrics.rejectedConnections
	perCommand := make(map[string]uint64, len(s.metrics.perCommand))
	for k, v := range s.metrics.perCommand {
		perCommand[k] = v
	}
	s.metrics.mu.Unlock()

	var b strings.Builder
	fmt.Fprintf(&b, "# Server\n")
	fmt.Fprintf(&b, "uptime_seconds:%d\n", int64(time.Since(startedAt).Seconds()))
	fmt.Fprintf(&b, "\n# Clients\n")
	fmt.Fprintf(&b, "connected_clients:%d\n", currentConnections)
	fmt.Fprintf(&b, "total_connections_received:%d\n", totalConnections)
	fmt.Fprintf(&b, "rejected_connections:%d\n", rejectedConnections)
	fmt.Fprintf(&b, "max_connections:%d\n", s.maxConns)
	fmt.Fprintf(&b, "\n# Stats\n")
	fmt.Fprintf(&b, "total_commands_processed:%d\n", totalCommands)
	fmt.Fprintf(&b, "total_errors:%d\n", totalErrors)
	fmt.Fprintf(&b, "\n# Memory\n")
	fmt.Fprintf(&b, "used_memory_approx:%d\n", s.store.ApproxMemory())
	fmt.Fprintf(&b, "maxmemory:%d\n", s.maxMemoryBytes)
	fmt.Fprintf(&b, "\n# Persistence\n")
	fmt.Fprintf(&b, "snapshot_enabled:%d\n", boolInt(s.snapshotPath != ""))
	fmt.Fprintf(&b, "aof_enabled:%d\n", boolInt(s.aof != nil))
	fmt.Fprintf(&b, "aof_fsync:%s\n", s.aofSync)
	fmt.Fprintf(&b, "\n# Commandstats\n")
	names := make([]string, 0, len(perCommand))
	for name := range perCommand {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(&b, "cmdstat_%s:calls=%d\n", strings.ToLower(name), perCommand[name])
	}
	return b.String()
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (s *Server) cmdSlowLog(args [][]byte) protocol.Reply {
	if len(args) < 1 {
		return protocol.ErrorReply{Msg: "ERR SLOWLOG requiere subcomando"}
	}
	switch strings.ToUpper(string(args[0])) {
	case "LEN":
		if len(args) != 1 {
			return protocol.ErrorReply{Msg: "ERR SLOWLOG LEN no recibe argumentos"}
		}
		s.metrics.mu.Lock()
		n := len(s.metrics.slowLog)
		s.metrics.mu.Unlock()
		return protocol.IntReply{N: int64(n)}
	case "RESET":
		if len(args) != 1 {
			return protocol.ErrorReply{Msg: "ERR SLOWLOG RESET no recibe argumentos"}
		}
		s.metrics.mu.Lock()
		s.metrics.slowLog = nil
		s.metrics.mu.Unlock()
		return protocol.StatusReply{Msg: "OK"}
	case "GET":
		limit := s.slowLogMaxLen
		if len(args) > 2 {
			return protocol.ErrorReply{Msg: "ERR SLOWLOG GET recibe cero o un límite"}
		}
		if len(args) == 2 {
			n, err := strconv.Atoi(string(args[1]))
			if err != nil || n < 0 {
				return protocol.ErrorReply{Msg: "ERR límite de SLOWLOG inválido"}
			}
			limit = n
		}
		return s.slowLogReply(limit)
	default:
		return protocol.ErrorReply{Msg: "ERR subcomando SLOWLOG desconocido"}
	}
}

func (s *Server) slowLogReply(limit int) protocol.Reply {
	s.metrics.mu.Lock()
	entries := make([]slowEntry, len(s.metrics.slowLog))
	copy(entries, s.metrics.slowLog)
	s.metrics.mu.Unlock()

	if limit < len(entries) {
		entries = entries[:limit]
	}
	elems := make([]protocol.Reply, len(entries))
	for i, entry := range entries {
		cmdElems := make([]protocol.Reply, len(entry.cmd))
		for j, part := range entry.cmd {
			cmdElems[j] = protocol.BulkReply{Value: []byte(part)}
		}
		elems[i] = protocol.ArrayReply{Elems: []protocol.Reply{
			protocol.IntReply{N: entry.id},
			protocol.IntReply{N: entry.at.Unix()},
			protocol.IntReply{N: entry.duration.Microseconds()},
			protocol.ArrayReply{Elems: cmdElems},
		}}
	}
	return protocol.ArrayReply{Elems: elems}
}
