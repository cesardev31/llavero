package server

import (
	"net"
	"sync"
	"time"

	"llavero/internal/protocol"
)

// client representa una conexión: escritura serializada + sus suscripciones.
// El mapa subs solo se accede desde la goroutine de la conexión.
type client struct {
	conn            net.Conn
	proto           protocol.Protocol
	writeTimeout    time.Duration
	writeMu         sync.Mutex
	subs            map[string]struct{}
	authed          bool
	closeAfterReply bool
}

func newClient(conn net.Conn, proto protocol.Protocol, writeTimeout time.Duration) *client {
	return &client{conn: conn, proto: proto, writeTimeout: writeTimeout, subs: make(map[string]struct{})}
}

// send serializa la escritura de una respuesta en la conexión.
func (c *client) send(reply protocol.Reply) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.writeTimeout > 0 {
		if err := c.conn.SetWriteDeadline(time.Now().Add(c.writeTimeout)); err != nil {
			return err
		}
		defer c.conn.SetWriteDeadline(time.Time{})
	}
	return c.proto.Encode(c.conn, reply)
}

// Deliver implementa pubsub.Subscriber: empuja un mensaje al cliente.
func (c *client) Deliver(channel string, payload []byte) {
	_ = c.send(protocol.ArrayReply{Elems: []protocol.Reply{
		protocol.BulkReply{Value: []byte("message")},
		protocol.BulkReply{Value: []byte(channel)},
		protocol.BulkReply{Value: payload},
	}})
}

// pubSubReply construye la confirmación de (un)subscribe.
func pubSubReply(kind, channel string, count int) protocol.Reply {
	return protocol.ArrayReply{Elems: []protocol.Reply{
		protocol.BulkReply{Value: []byte(kind)},
		protocol.BulkReply{Value: []byte(channel)},
		protocol.IntReply{N: int64(count)},
	}}
}

// cmdSubscribe suscribe al cliente a los canales dados y confirma cada uno.
// Envía las respuestas directamente y devuelve nil.
func (s *Server) cmdSubscribe(c *client, args [][]byte) protocol.Reply {
	if len(args) < 1 {
		return protocol.ErrorReply{Msg: "ERR SUBSCRIBE requiere al menos 1 canal"}
	}
	for _, ch := range args {
		channel := string(ch)
		s.broker.Subscribe(c, channel)
		c.subs[channel] = struct{}{}
		_ = c.send(pubSubReply("subscribe", channel, len(c.subs)))
	}
	return nil
}

// cmdUnsubscribe da de baja al cliente de los canales dados (o de todos si no
// se pasan). Envía las respuestas directamente y devuelve nil.
func (s *Server) cmdUnsubscribe(c *client, args [][]byte) protocol.Reply {
	channels := make([]string, 0, len(args))
	if len(args) == 0 {
		for ch := range c.subs {
			channels = append(channels, ch)
		}
	} else {
		for _, ch := range args {
			channels = append(channels, string(ch))
		}
	}
	if len(channels) == 0 {
		_ = c.send(protocol.ArrayReply{Elems: []protocol.Reply{
			protocol.BulkReply{Value: []byte("unsubscribe")},
			protocol.BulkReply{Null: true},
			protocol.IntReply{N: 0},
		}})
		return nil
	}
	for _, channel := range channels {
		s.broker.Unsubscribe(c, channel)
		delete(c.subs, channel)
		_ = c.send(pubSubReply("unsubscribe", channel, len(c.subs)))
	}
	return nil
}

// cmdPublish entrega un mensaje a los suscriptores del canal.
func (s *Server) cmdPublish(args [][]byte) protocol.Reply {
	if len(args) != 2 {
		return protocol.ErrorReply{Msg: "ERR PUBLISH requiere canal y mensaje"}
	}
	n := s.broker.Publish(string(args[0]), args[1])
	return protocol.IntReply{N: int64(n)}
}

// unsubscribeAll da de baja al cliente de todos sus canales (al desconectar).
func (s *Server) unsubscribeAll(c *client) {
	for ch := range c.subs {
		s.broker.Unsubscribe(c, ch)
	}
}
