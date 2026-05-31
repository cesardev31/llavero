# Llavero Fase 13 — Observabilidad

**Goal:** Añadir introspección operativa básica: métricas internas, comandos de
consulta, logs estructurados y slow log simple.

**Estado:** implementado en este corte.

## Decisiones

- `INFO [section]` devuelve un bulk string estilo Redis con secciones:
  `Server`, `Clients`, `Stats`, `Memory`, `Persistence` y `Commandstats`.
- `STATS` es un alias sin argumentos que devuelve la misma fotografía que
  `INFO`.
- Se cuentan comandos, errores, conexiones aceptadas, conexiones actuales,
  conexiones rechazadas y llamadas por comando.
- `SLOWLOG LEN`, `SLOWLOG GET [n]` y `SLOWLOG RESET` quedan implementados.
- `-slowlog-threshold` configura la latencia mínima para registrar comandos
  lentos. `0` desactiva el slow log.
- `-slowlog-max-len` limita el número de entradas retenidas.
- Los logs de comandos usan formato `key=value` con comando, remoto, latencia y
  si hubo error.
- El slow log redacta argumentos de `AUTH`.

## Archivos

- `internal/server/observability.go`
- `internal/server/server.go`
- `internal/server/server_test.go`
- `cmd/llavero/main.go`
- `README.md`

## Verificación esperada

```bash
go test ./internal/server -race -v
go test ./... -race
go vet ./...
go build ./...
```

## Próximo paso

La fase siguiente recomendada es **Fase 14 — Compatibilidad Redis más completa**:
edge cases RESP, nombres de errores más compatibles y comandos auxiliares para
clientes reales.
