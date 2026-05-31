# Llavero Fase 10 — Durabilidad fuerte (AOF/WAL)

**Goal:** Añadir un append-only file para recuperar escrituras confirmadas tras
reinicio, con política de `fsync` configurable y replay probado.

**Estado:** implementado en este corte.

## Decisiones

- AOF se activa con `-aof path`.
- `-aof-fsync` acepta `always`, `everysec` o `no`.
- Si `-aof` se pasa sin `-snapshot`, el binario desactiva el snapshot por
  defecto para evitar duplicar modos de persistencia.
- `SnapshotPath` y `AOFPath` son excluyentes en `server.NewWithOptions` hasta
  implementar compactación/rewrite.
- `EXPIRE` se registra como `PEXPIREAT` para conservar expiraciones absolutas
  durante replay.
- El AOF registra comandos mutantes en RESP2 y los reproduce con el dispatcher
  normal al arrancar.

## Archivos

- `internal/persistence/aof.go`
- `internal/persistence/aof_test.go`
- `internal/server/aof.go`
- `internal/server/server.go`
- `internal/server/server_test.go`
- `internal/store/store.go`
- `internal/command/command.go`
- `internal/command/handlers.go`
- `cmd/llavero/main.go`
- `README.md`

## Verificación esperada

```bash
go test ./internal/persistence ./internal/server ./internal/store ./internal/command -race -v
go test ./... -race
go vet ./...
go build ./...
```

## Próximo paso

La fase siguiente recomendada es **Fase 11 — Autenticación y superficie segura**:
`AUTH`, binding documentado, TLS opcional y preparación para límites por cliente.
