# Llavero Fase 14 — Compatibilidad Redis más completa

**Goal:** Mejorar la compatibilidad práctica con clientes Redis reales y casos
RESP esperados, sin salir de la librería estándar.

**Estado:** implementado en este corte.

## Decisiones

- RESP acepta comandos inline simples (`PING hola\r\n`) además de arrays bulk.
- RESP puede serializar arrays nulos (`*-1`).
- Se agregan comandos auxiliares de compatibilidad:
  - `HELLO 2 [AUTH default password]`
  - `COMMAND`, `COMMAND COUNT`, `COMMAND DOCS`, `COMMAND INFO ...`
  - `CLIENT ID`, `CLIENT SETINFO`, `CLIENT SETNAME`, `CLIENT GETNAME`,
    `CLIENT INFO`, `CLIENT LIST`
  - `SELECT 0`
  - `ECHO message`
  - `QUIT`
- `HELLO AUTH` autentica la conexión cuando `-requirepass` está activo.
- `QUIT` responde `OK` y cierra la conexión.
- `COMMAND` devuelve metadata mínima suficiente para clientes que introspectan
  capacidades.

## Archivos

- `internal/protocol/protocol.go`
- `internal/protocol/resp.go`
- `internal/protocol/miniresp.go`
- `internal/protocol/resp_test.go`
- `internal/server/compat.go`
- `internal/server/pubsub.go`
- `internal/server/server.go`
- `internal/server/server_test.go`
- `README.md`

## Verificación esperada

```bash
go test ./internal/protocol ./internal/server -race -v
go test ./... -race
go vet ./...
go build ./...
```

## Próximo paso

La fase siguiente recomendada es **Fase 15 — Operación**:
health check, configuración por archivo/env, Dockerfile, unidad systemd y
shutdown con drenaje de conexiones.
