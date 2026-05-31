# Llavero

Llavero es un almacén clave-valor en memoria escrito en Go, inspirado en Redis.
Expone un servidor TCP con protocolo RESP2 y soporta strings, listas, hashes,
sets, TTL, snapshots a disco y pub/sub.

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

## CLI

Llavero incluye un cliente propio para enviar comandos al servidor:

```bash
go run ./cmd/llavero-cli PING
go run ./cmd/llavero-cli SET saludo "hola mundo"
go run ./cmd/llavero-cli GET saludo
```

La dirección por defecto es `127.0.0.1:6380`; se puede cambiar con `-addr`:

```bash
go run ./cmd/llavero-cli -addr 127.0.0.1:16380 PING
```

Sin argumentos entra en modo interactivo:

```bash
go run ./cmd/llavero-cli
llavero> SET k v
llavero> GET k
llavero> quit
```

Para pub/sub, `SUBSCRIBE` queda leyendo mensajes hasta cortar el proceso:

```bash
go run ./cmd/llavero-cli SUBSCRIBE news
go run ./cmd/llavero-cli PUBLISH news hola
```

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

Pub/sub:

- `SUBSCRIBE channel...`
- `UNSUBSCRIBE [channel...]`
- `PUBLISH channel message`

## Protocolo RESP2

Una petición se envía como `*N\r\n` seguido de `N` partes
`$len\r\n<bytes>\r\n`.
Ejemplo para `SET nombre cesar`:

```text
*3\r\n$3\r\nSET\r\n$6\r\nnombre\r\n$5\r\ncesar\r\n
```

Las respuestas usan prefijos estilo Redis: `+` para estado, `-` para error,
`:` para entero, `$` para bulk y `*` para array.
