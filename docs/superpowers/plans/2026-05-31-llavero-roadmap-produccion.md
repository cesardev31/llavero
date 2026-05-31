# Llavero — Roadmap hacia producción

> Plan vivo. Mantener el trabajo por fases pequeñas, con tests y smoke cuando
> aplique, siguiendo la cadencia de los planes anteriores.

## Orden propuesto

1. **Fase 10 — Durabilidad fuerte (AOF/WAL)**
   - Append-only file para comandos mutantes.
   - `fsync` configurable: `always`, `everysec`, `no`.
   - Replay al arranque.
   - TTLs persistidos con expiración absoluta.
   - Snapshot y AOF quedan excluyentes hasta tener compactación/rewrite.

2. **Fase 11 — Autenticación y superficie segura**
   - `AUTH` básico con secreto configurado por flag/env.
   - Binding seguro por defecto documentado.
   - TLS opcional con cert/key.
   - Errores compatibles para comandos antes de autenticación.

3. **Fase 12 — Límites de recursos**
   - Máximo de conexiones.
   - Timeouts de lectura/escritura.
   - Límite de memoria aproximado.
   - Backpressure para clientes lentos y pub/sub.
   - Límites configurables por flags.

4. **Fase 13 — Observabilidad**
   - `INFO`/`STATS` básicos.
   - Métricas internas: comandos, conexiones, memoria aproximada, AOF, snapshots.
   - Logs estructurados sin dependencias externas.
   - Latencias y slow log simple.

5. **Fase 14 — Compatibilidad Redis más completa**
   - Edge cases RESP esperados por clientes reales.
   - Respuestas y nombres de errores más compatibles.
   - Comandos auxiliares útiles (`COMMAND`, `CLIENT`, variantes TTL).
   - Tests contra `redis-cli` si está disponible.

6. **Fase 15 — Operación**
   - Health check.
   - Configuración por archivo/env.
   - Dockerfile.
   - Unidad systemd de ejemplo.
   - Shutdown más robusto con drenaje de conexiones.

7. **Fase 16 — Pruebas de estrés**
   - Benchmarks.
   - Soak tests.
   - Fuzzing del protocolo RESP.
   - Pruebas con miles de conexiones y pub/sub.

8. **Fase 17 — Replicación/HA**
   - Réplica read-only inicial.
   - Stream de cambios desde AOF.
   - Promoción manual.
   - Failover automático queda fuera hasta tener operación y observabilidad sólidas.

## Regla de avance

Cada fase debe terminar con:

- tests unitarios o de integración específicos;
- `go test ./... -race`;
- `go vet ./...`;
- `go build ./...`;
- documentación mínima de flags/comandos nuevos;
- commit pequeño con el alcance de la fase.
