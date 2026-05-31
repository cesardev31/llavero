# Llavero

Llavero es un almacén clave-valor en memoria escrito en Go, inspirado en Redis.
Expone un servidor TCP con protocolo mini-RESP propio y soporta strings, listas,
hashes, sets, TTL y snapshots a disco.

## Ejecutar

```bash
go run ./cmd/llavero
```

Flags disponibles:

```bash
go run ./cmd/llavero \
  -addr :6380 \
  -snapshot llavero.snapshot \
  -save-interval 30s
```

- `-addr`: dirección TCP de escucha.
- `-snapshot`: archivo usado para cargar al arrancar y guardar con `SAVE`.
  Si queda vacío, la persistencia queda desactivada.
- `-save-interval`: intervalo de snapshot automático. `0` lo desactiva.

## Comandos

Strings y TTL:

- `PING [mensaje]`
- `GET key`
- `SET key value`
- `DEL key...`
- `EXISTS key`
- `EXPIRE key segundos`
- `TTL key`
- `PERSIST key`

Listas:

- `LPUSH key value...`
- `RPUSH key value...`
- `LPOP key`
- `RPOP key`
- `LLEN key`
- `LRANGE key start stop`

Hashes:

- `HSET key field value`
- `HGET key field`
- `HDEL key field...`
- `HGETALL key`
- `HLEN key`

Sets:

- `SADD key member...`
- `SREM key member...`
- `SISMEMBER key member`
- `SMEMBERS key`
- `SCARD key`

Persistencia:

- `SAVE`

## Protocolo mini-RESP

Una petición se envía como `*N\n` seguido de `N` partes `$len\n<bytes>\n`.
Ejemplo para `SET nombre cesar`:

```text
*3
$3
SET
$6
nombre
$5
cesar
```

Las respuestas usan prefijos estilo Redis: `+` para estado, `-` para error,
`:` para entero, `$` para bulk y `*` para array.
