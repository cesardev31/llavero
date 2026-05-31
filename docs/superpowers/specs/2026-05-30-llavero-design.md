# Llavero — Diseño

**Fecha:** 2026-05-30
**Estado:** Aprobado (diseño global, fases 1–5)

## Resumen

Llavero es un almacén clave-valor en memoria escrito en Go, inspirado en Redis.
Tiene dos objetivos por igual:

1. **Aprender** cómo funciona por dentro un sistema así: redes, concurrencia,
   estructuras de datos, expiración y persistencia, construido por fases.
2. Terminar con una **herramienta sólida y usable** en proyectos reales (cache,
   sesiones, contadores), con una arquitectura preparada para crecer.

El servidor expone un socket TCP. Para la primera versión usa un **protocolo de
texto propio y sencillo**, con la capa de protocolo aislada detrás de una
interfaz para poder añadir compatibilidad **RESP** (clientes reales como
`redis-cli` / `go-redis`) más adelante sin reescribir el núcleo.

## Objetivos y no-objetivos

**Objetivos (fases 1–5):**
- Servidor TCP concurrente, una goroutine por conexión.
- Protocolo propio simple e intercambiable.
- Comandos núcleo de strings: `GET`, `SET`, `DEL`, `EXISTS`.
- Expiración: `EXPIRE`, `TTL`, `PERSIST`, con borrado lazy + activo.
- Estructuras de datos: listas, hashes, sets.
- Persistencia por snapshot a disco + carga al arrancar.
- Almacén escalable mediante **sharding + locks**.

**No-objetivos (por ahora, posibles mejoras futuras):**
- Compatibilidad RESP (diseñada como punto de extensión, no implementada).
- Pub/Sub y transacciones (`MULTI`/`EXEC`).
- AOF (log de operaciones) y replicación / clustering.
- Autenticación y TLS.

## Arquitectura en capas

Cada capa es una unidad aislada con una interfaz clara, entendible y testeable
por separado, y reemplazable sin romper a las demás.

```
Cliente TCP
   │
[ Red ]          accept loop → 1 goroutine por conexión
   │
[ Protocolo ]    interfaz Protocol{ Parse, Encode } ← intercambiable (propio hoy, RESP mañana)
   │
[ Comandos ]     dispatcher: nombre de comando → handler
   │
[ Almacén ]      engine con N shards (hash(key)%N), cada shard con su RWMutex
   │
[ Valores ]      tipos: String, List, Hash, Set  (+ metadatos de TTL)
   │
[ Persistencia ] snapshot a disco + carga al arrancar
```

### Estructura de paquetes propuesta

```
llavero/
  cmd/llavero/main.go        # arranque del servidor
  internal/server/           # capa de red (TCP, accept loop, manejo de conexión)
  internal/protocol/         # interfaz Protocol + implementación del protocolo propio
  internal/command/          # dispatcher y handlers de comandos
  internal/store/            # engine con sharding, shards, RWMutex
  internal/value/            # tipos de valor: String, List, Hash, Set + TTL
  internal/persistence/      # snapshot: serializar/cargar
  docs/superpowers/specs/    # specs de diseño
```

## Interfaces clave

```go
// Protocolo: aísla cómo se leen/escriben los bytes del cliente.
type Protocol interface {
    Parse(r *bufio.Reader) (Command, error)   // bytes → comando
    Encode(w io.Writer, resp Reply) error     // respuesta → bytes
}

// Comando ya parseado.
type Command struct {
    Name string
    Args [][]byte
}

// Handler de un comando concreto.
type Handler func(s *store.Store, args [][]byte) Reply
```

El dispatcher mantiene un `map[string]Handler`. Añadir un comando nuevo es
registrar un handler, sin tocar red ni protocolo.

## Flujo de datos

```
conn → Protocol.Parse → Command{name,args} → dispatcher → shard.Lock
     → operación → Reply → Protocol.Encode → conn
```

## Concurrencia: sharding + locks

- El almacén se divide en N shards (configurable, p.ej. 256).
- Cada clave se asigna al shard `hash(key) % N` (hash estable, p.ej. FNV).
- Cada shard tiene su propio `sync.RWMutex` y su propio `map[string]*Entry`.
- Lecturas usan `RLock`; escrituras usan `Lock`.
- Dos clientes que operan sobre claves de shards distintos no compiten por el
  mismo lock → escala en máquinas multi-core.
- Se valida con `go test -race`.

## TTL / expiración

Combina dos mecanismos, como Redis:
- **Lazy:** al acceder a una clave vencida, se borra y se trata como inexistente.
- **Activa:** una goroutine de fondo muestrea claves periódicamente y limpia las
  vencidas, evitando fugas de memoria de claves que nadie vuelve a leer.

Cada `Entry` guarda su valor y, opcionalmente, un instante de expiración.

## Persistencia

- **Snapshot:** se serializa el estado de todos los shards a un archivo en disco
  de forma periódica y/o bajo demanda.
- **Carga:** al arrancar, si existe el snapshot, se reconstruye el almacén.
- Se empieza con un **formato propio simple** (longitud + bytes por campo) para
  entender la serialización a bajo nivel, en lugar de delegar en una librería.
- AOF (log de operaciones append-only) queda como mejora futura opcional.

## Manejo de errores

- **Errores de protocolo** (entrada malformada): se responde error al cliente y,
  si la conexión queda en estado inconsistente, se cierra solo esa conexión.
- **Errores de comando** (comando inexistente, aridad incorrecta, tipo de valor
  equivocado): se devuelve un error legible al cliente.
- Ninguna goroutine de conexión puede derribar el servidor ni afectar a otras
  conexiones (recuperación de pánico por conexión).

## Estrategia de testing

TDD por capa:
- **protocol:** tests de parse/encode (entradas válidas e inválidas).
- **value:** tests de cada estructura (String, List, Hash, Set).
- **store:** operaciones + concurrencia con `go test -race`.
- **command:** cada handler con su tabla de casos.
- **persistencia:** round-trip snapshot → carga.
- **integración:** cliente TCP real contra el servidor (end-to-end por fase).

## Fases incrementales

Cada fase es un ciclo spec → plan → implementación. Este documento es el diseño
global; el primer plan de implementación abordará la Fase 1.

1. **Esqueleto:** servidor TCP + `PING`/`PONG` + goroutine por conexión.
2. **Núcleo KV:** protocolo propio + `GET`/`SET`/`DEL`/`EXISTS` + store con sharding.
3. **TTL:** `EXPIRE`/`TTL`/`PERSIST` + expiración lazy y activa.
4. **Estructuras:** listas, hashes, sets con sus comandos.
5. **Persistencia:** snapshot + carga al arrancar.
6. **Futuro (fuera de este spec):** capa RESP, pub/sub, transacciones, AOF.

## Decisiones abiertas para implementación

- Valor concreto de N (número de shards) y si se hace configurable por flag.
- Intervalo de snapshot y de la barrida de expiración activa.
- Función de hash exacta para el sharding (candidata: FNV-1a de `hash/fnv`).
