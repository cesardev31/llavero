package server

import (
	"crypto/subtle"
	"log"
	"sync"
	"time"

	"llavero/internal/protocol"
)

// authLimiter lleva la cuenta de intentos fallidos de AUTH por IP.
type authLimiter struct {
	mu       sync.Mutex
	failures map[string]int
	blocked  map[string]time.Time
}

func newAuthLimiter() *authLimiter {
	return &authLimiter{
		failures: make(map[string]int),
		blocked:  make(map[string]time.Time),
	}
}

// maxAuthAttempts es el máximo de intentos fallidos antes de bloquear.
const maxAuthAttempts = 5

// authBlockDuration es el tiempo de bloqueo tras superar el límite.
const authBlockDuration = 30 * time.Second

func (al *authLimiter) check(remote string) bool {
	al.mu.Lock()
	defer al.mu.Unlock()
	if until, ok := al.blocked[remote]; ok {
		if time.Now().Before(until) {
			return false
		}
		delete(al.blocked, remote)
		delete(al.failures, remote)
	}
	return true
}

func (al *authLimiter) recordFailure(remote string) {
	al.mu.Lock()
	defer al.mu.Unlock()
	al.failures[remote]++
	if al.failures[remote] >= maxAuthAttempts {
		al.blocked[remote] = time.Now().Add(authBlockDuration)
		log.Printf("event=auth_blocked remote=%q attempts=%d block_duration=%s", remote, al.failures[remote], authBlockDuration)
	}
}

func (al *authLimiter) recordSuccess(remote string) {
	al.mu.Lock()
	defer al.mu.Unlock()
	delete(al.failures, remote)
	delete(al.blocked, remote)
}

func (s *Server) cmdAuth(c *client, args [][]byte) protocol.Reply {
	if s.authPassword == "" {
		return protocol.ErrorReply{Msg: "ERR AUTH not required"}
	}
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR AUTH requires exactly one argument"}
	}
	remote := c.conn.RemoteAddr().String()
	if !s.authLimit.check(remote) {
		return protocol.ErrorReply{Msg: "ERR too many AUTH failures, try again later"}
	}
	if subtle.ConstantTimeCompare(args[0], []byte(s.authPassword)) != 1 {
		s.authLimit.recordFailure(remote)
		log.Printf("event=auth_failed remote=%q", remote)
		return protocol.ErrorReply{Msg: "ERR invalid password"}
	}
	s.authLimit.recordSuccess(remote)
	c.authed = true
	return protocol.StatusReply{Msg: "OK"}
}

func (s *Server) authRequired(c *client) bool {
	return s.authPassword != "" && !c.authed
}
