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
  -config deploy/llavero.conf.example \
  -addr 127.0.0.1:6380 \
  -snapshot llavero.snapshot \
  -save-interval 30s
```

- `-config`: archivo `key=value` con configuración base.
- `-addr`: dirección TCP de escucha. Por defecto escucha solo en
  `127.0.0.1:6380`; usa una IP explícita o `:6380` solo si quieres exponerlo
  fuera de localhost.
- `-snapshot`: archivo usado para cargar al arrancar y guardar con `SAVE`.
  Si queda vacío, la persistencia queda desactivada.
- `-save-interval`: intervalo de snapshot automático. `0` lo desactiva.
- `-aof`: archivo append-only para recuperar escrituras confirmadas.
- `-aof-fsync`: política de sincronización del AOF: `always`, `everysec` o
  `no`. Por defecto usa `always`.
- `-requirepass`: contraseña requerida para `AUTH`; también puede venir de
  `LLAVERO_REQUIREPASS`.
- `-tls-cert` y `-tls-key`: habilitan TLS con certificado y llave PEM.
- `-max-connections`: máximo de conexiones simultáneas. `0` lo desactiva.
- `-read-timeout`: timeout de lectura por comando. `0` lo desactiva.
- `-write-timeout`: timeout de escritura por respuesta/pubsub. `0` lo desactiva.
- `-max-memory`: límite aproximado de bytes para claves y valores vivos. `0`
  lo desactiva.
- `-slowlog-threshold`: latencia mínima para registrar comandos lentos. `0` lo
  desactiva.
- `-slowlog-max-len`: máximo de entradas retenidas en `SLOWLOG`.
- `-shutdown-timeout`: tiempo máximo para drenar conexiones durante apagado.

Las variables `LLAVERO_*` equivalentes (`LLAVERO_ADDR`,
`LLAVERO_REQUIREPASS`, `LLAVERO_AOF`, etc.) se aplican después del archivo de
configuración y antes de los flags explícitos.

Snapshot y AOF todavía son modos excluyentes. Si se usa `-aof` sin pasar
`-snapshot`, el servidor desactiva el snapshot por defecto automáticamente:

```bash
go run ./cmd/llavero -aof appendonly.aof -aof-fsync always
```

Ejemplo con autenticación:

```bash
LLAVERO_REQUIREPASS=secreto go run ./cmd/llavero
go run ./cmd/llavero-cli -auth secreto PING
```

Ejemplo con límites de recursos:

```bash
go run ./cmd/llavero \
  -max-connections 1000 \
  -read-timeout 30s \
  -write-timeout 5s \
  -max-memory 1073741824 \
  -slowlog-threshold 10ms
```

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

Si el servidor exige `AUTH`, pasa `-auth`:

```bash
go run ./cmd/llavero-cli -auth secreto SET saludo "hola mundo"
```

Para servidores con TLS:

```bash
go run ./cmd/llavero-cli -tls -tls-skip-verify PING
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
- `AUTH password`
- `HEALTH`
- `HELLO 2 [AUTH default password]`
- `COMMAND [COUNT|DOCS|INFO command...]`
- `CLIENT ID|SETINFO|SETNAME|GETNAME|INFO|LIST`
- `SELECT 0`
- `ECHO message`
- `QUIT`
- `INFO [section]`
- `STATS`
- `SLOWLOG GET [n]`
- `SLOWLOG LEN`
- `SLOWLOG RESET`
- `GET key`
- `SET key value`
- `DEL key...`
- `EXISTS key`
- `EXPIRE key segundos`
- `PEXPIREAT key unix_ms`
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
