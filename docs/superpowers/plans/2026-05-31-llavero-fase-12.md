# Llavero Fase 12 — Límites de recursos

**Goal:** Añadir límites operativos básicos para que una instancia pueda
protegerse frente a exceso de conexiones, clientes lentos y crecimiento de
memoria.

**Estado:** implementado en este corte.

## Decisiones

- `-max-connections` limita conexiones simultáneas. `0` lo desactiva.
- `-read-timeout` fija un deadline por comando leído. `0` lo desactiva.
- `-write-timeout` fija deadline para respuestas y mensajes pub/sub. `0` lo
  desactiva. Esto actúa como backpressure simple para clientes lentos.
- `-max-memory` limita de forma aproximada el tamaño vivo de claves y valores.
  No incluye toda la sobrecarga interna de mapas/slices de Go; es un límite
  conservador para cortar crecimiento antes de aplicar mutaciones.
- Las opciones negativas se rechazan al crear el servidor.

## Archivos

- `internal/store/memory.go`
- `internal/store/memory_test.go`
- `internal/server/server.go`
- `internal/server/aof.go`
- `internal/server/pubsub.go`
- `internal/server/server_test.go`
- `cmd/llavero/main.go`
- `README.md`

## Verificación esperada

```bash
go test ./internal/store ./internal/server -race -v
go test ./... -race
go vet ./...
go build ./...
```

## Próximo paso

La fase siguiente recomendada es **Fase 13 — Observabilidad**:
`INFO`/`STATS`, métricas internas, logs estructurados sin dependencias y slow log
simple.
