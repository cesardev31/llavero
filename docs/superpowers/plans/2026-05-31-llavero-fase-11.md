# Llavero Fase 11 — Autenticación y superficie segura

**Goal:** Reducir la superficie insegura por defecto y añadir autenticación
básica con `AUTH`, más TLS opcional sin dependencias externas.

**Estado:** implementado en este corte.

## Decisiones

- El binding por defecto cambia de `:6380` a `127.0.0.1:6380`.
- `-requirepass` activa autenticación por conexión.
- `LLAVERO_REQUIREPASS` puede configurar la contraseña sin pasarla por argv.
- Si hay contraseña configurada, solo `AUTH <password>` se acepta antes de
  autenticar; el resto responde `NOAUTH Authentication required.`.
- Si no hay contraseña configurada, `AUTH` responde `ERR AUTH no requerido`.
- TLS se activa con `-tls-cert` y `-tls-key`; ambos son obligatorios juntos.
- La CLI soporta `-auth`, `-tls`, `-tls-skip-verify` y `-tls-server-name`.

## Archivos

- `internal/server/auth.go`
- `internal/server/pubsub.go`
- `internal/server/server.go`
- `internal/server/server_test.go`
- `cmd/llavero/main.go`
- `cmd/llavero-cli/main.go`
- `README.md`

## Verificación esperada

```bash
go test ./internal/server -race -v
go test ./... -race
go vet ./...
go build ./...
```

## Próximo paso

La fase siguiente recomendada es **Fase 12 — Límites de recursos**:
máximo de conexiones, timeouts de lectura/escritura, límite de memoria
aproximado y protección frente a clientes lentos.
