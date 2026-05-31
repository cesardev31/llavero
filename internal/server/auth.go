package server

import (
	"crypto/subtle"

	"llavero/internal/protocol"
)

func (s *Server) cmdAuth(c *client, args [][]byte) protocol.Reply {
	if s.authPassword == "" {
		return protocol.ErrorReply{Msg: "ERR AUTH no requerido"}
	}
	if len(args) != 1 {
		return protocol.ErrorReply{Msg: "ERR AUTH requiere 1 argumento"}
	}
	if subtle.ConstantTimeCompare(args[0], []byte(s.authPassword)) != 1 {
		return protocol.ErrorReply{Msg: "ERR contraseña inválida"}
	}
	c.authed = true
	return protocol.StatusReply{Msg: "OK"}
}

func (s *Server) authRequired(c *client) bool {
	return s.authPassword != "" && !c.authed
}
