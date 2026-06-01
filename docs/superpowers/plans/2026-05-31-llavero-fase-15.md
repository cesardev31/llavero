# Llavero Fase 15 — Operación

**Goal:** Añadir piezas básicas para operar Llavero como servicio: health check,
configuración por archivo/env, artefactos de despliegue y apagado con drenaje.

**Estado:** implementado en este corte.

## Decisiones

- `HEALTH` responde `+OK` y queda incluido en `COMMAND`.
- `cmd/llavero` acepta `-config` con archivo `key=value`.
- La precedencia de configuración es:
  1. defaults;
  2. archivo `-config`;
  3. variables `LLAVERO_*`;
  4. flags explícitos.
- `snapshot=` vacío en config desactiva snapshots.
- `ShutdownTimeout` permite esperar conexiones activas en `Close`.
- `-shutdown-timeout` configura el drenaje. Default: `5s`.
- Se agregan:
  - `Dockerfile`;
  - `deploy/llavero.conf.example`;
  - `deploy/llavero.service`.

## Verificación esperada

```bash
go test ./internal/config ./internal/server -race -v
go test ./... -race
go vet ./...
go build ./...
```

## Próximo paso

La fase siguiente recomendada es **Fase 16 — Pruebas de estrés**:
benchmarks, soak tests, fuzzing RESP y pruebas con muchas conexiones.
