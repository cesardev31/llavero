# Llavero Fase 6 — Plan de Implementación (Apagado limpio + snapshot final)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Que Llavero se apague de forma ordenada ante SIGINT/SIGTERM y guarde un snapshot final, cerrando el agujero de durabilidad por defecto.

**Architecture:** `Server.Serve()` distingue un cierre ordenado (canal `stop` cerrado) de un error real y devuelve `nil` en ese caso. Se añade `Server.Save()` que persiste un snapshot si hay `snapshotPath`. `main.go` captura señales del SO y orquesta el apagado: `Save()` + `Close()`.

**Tech Stack:** Go 1.26, librería estándar (`os/signal`, `syscall`, `errors`, `net`).

## Estado actual relevante

`internal/server/server.go` ya tiene: `Server` con campos `stop chan struct{}`, `closeOnce sync.Once`, `snapshotPath string`; `NewWithOptions(Options)`; `Close()` (cierra `stop` con `sync.Once` y el listener); `Serve()` (lanza `expireLoop`/`saveLoop` y hace el accept loop, devolviendo el error de `Accept`); y usa `persistence.Save(path, store)`. `cmd/llavero/main.go` crea el server con flags `--addr`, `--snapshot`, `--save-interval`, llama `Listen()` y `Serve()`, y hace `log.Fatalf` si `Serve()` devuelve error.

## Estructura de archivos

- `internal/server/server.go` — `Serve()` retorna `nil` en cierre ordenado; nuevo método `Save()`.
- `internal/server/server_test.go` — tests de cierre ordenado y de `Save()`.
- `cmd/llavero/main.go` — manejo de señales + apagado ordenado.

---

### Task 1: Serve() retorna nil en cierre ordenado + método Save()

**Files:**
- Modify: `internal/server/server.go`
- Modify: `internal/server/server_test.go`

- [ ] **Step 1: Añadir los tests que fallan**

APPEND a `internal/server/server_test.go`:
```go
func TestServeReturnsNilOnGracefulClose(t *testing.T) {
	s := New("127.0.0.1:0")
	if err := s.Listen(); err != nil {
		t.Fatalf("Listen devolvió error: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- s.Serve() }()

	// dar un instante a que arranque el accept loop, luego cerrar
	time.Sleep(20 * time.Millisecond)
	s.Close()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve tras cierre ordenado devolvió %v, quería nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve no retornó tras Close")
	}
}

func TestSaveWritesSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dump.llavero")
	s, err := NewWithOptions(Options{Addr: "127.0.0.1:0", SnapshotPath: path})
	if err != nil {
		t.Fatalf("NewWithOptions devolvió error: %v", err)
	}
	s.store.Set("k", []byte("v"))

	if err := s.Save(); err != nil {
		t.Fatalf("Save devolvió error: %v", err)
	}

	loaded := store.New(16)
	if err := persistence.Load(path, loaded); err != nil {
		t.Fatalf("Load -> %v", err)
	}
	if got, ok, err := loaded.Get("k"); err != nil || !ok || string(got) != "v" {
		t.Fatalf("snapshot GET -> %q %v %v", got, ok, err)
	}
}

func TestSaveWithoutSnapshotPathIsNoop(t *testing.T) {
	s := New("127.0.0.1:0") // sin SnapshotPath
	s.store.Set("k", []byte("v"))
	if err := s.Save(); err != nil {
		t.Fatalf("Save sin snapshotPath debería ser no-op, devolvió %v", err)
	}
}
```

Nota: `server_test.go` ya importa `bufio`, `net`, `strings`, `testing`, `path/filepath`, `llavero/internal/persistence` y `llavero/internal/store`. Falta `time`: añadir `"time"` al bloque de imports de `server_test.go` si no está ya presente.

- [ ] **Step 2: Ejecutar y verificar que falla**

Run: `go test ./internal/server/ -run 'TestServeReturnsNilOnGracefulClose|TestSaveWritesSnapshot|TestSaveWithoutSnapshotPathIsNoop' -v`
Expected: FALLA al compilar con "s.Save undefined". (Si `time` ya estaba importado, el único fallo es `s.Save undefined`.)

- [ ] **Step 3: Modificar `Serve()` en `internal/server/server.go`**

Reemplazar el cuerpo del bucle de aceptación. El `Serve()` actual es:
```go
func (s *Server) Serve() error {
	go s.expireLoop()
	if s.snapshotPath != "" && s.saveInterval > 0 {
		go s.saveLoop()
	}
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return err
		}
		go s.handleConn(conn)
	}
}
```
Reemplazarlo por:
```go
func (s *Server) Serve() error {
	go s.expireLoop()
	if s.snapshotPath != "" && s.saveInterval > 0 {
		go s.saveLoop()
	}
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			// si el cierre fue ordenado (stop cerrado), no es un error
			select {
			case <-s.stop:
				return nil
			default:
				return err
			}
		}
		go s.handleConn(conn)
	}
}
```

- [ ] **Step 4: Añadir el método `Save()` en `internal/server/server.go`**

Justo después de `Close()`, añadir:
```go
// Save guarda un snapshot del store si hay snapshotPath configurado.
// Es no-op (sin error) si no se configuró persistencia.
func (s *Server) Save() error {
	if s.snapshotPath == "" {
		return nil
	}
	return persistence.Save(s.snapshotPath, s.store)
}
```

- [ ] **Step 5: Ejecutar y verificar que pasa (con -race)**

Run: `go test ./internal/server/ -race -v`
Expected: PASS en todos los tests (los previos + los 3 nuevos), sin data races.

- [ ] **Step 6: Commit**

```bash
git add internal/server/server.go internal/server/server_test.go
git -c user.email="developert@orvixapp.com" -c user.name="cesar" commit -m "feat: Serve retorna nil en cierre ordenado y método Server.Save"
```

---

### Task 2: Manejo de señales y apagado ordenado en main.go

**Files:**
- Modify: `cmd/llavero/main.go`

(No hay tests automáticos para `package main`; se valida en la Tarea 3 con prueba de humo.)

- [ ] **Step 1: Reescribir `cmd/llavero/main.go`**

Reemplazar TODO el contenido por:
```go
package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"llavero/internal/server"
)

func main() {
	addr := flag.String("addr", ":6380", "dirección TCP de escucha")
	snapshot := flag.String("snapshot", "llavero.snapshot", "archivo de snapshot; vacío desactiva persistencia")
	saveInterval := flag.Duration("save-interval", 0, "intervalo de snapshot automático; 0 lo desactiva")
	flag.Parse()

	s, err := server.NewWithOptions(server.Options{
		Addr:         *addr,
		SnapshotPath: *snapshot,
		SaveInterval: *saveInterval,
	})
	if err != nil {
		log.Fatalf("no se pudo cargar snapshot: %v", err)
	}
	if err := s.Listen(); err != nil {
		log.Fatalf("no se pudo escuchar: %v", err)
	}
	log.Printf("Llavero escuchando en %s", s.Addr())

	// apagado ordenado ante SIGINT/SIGTERM: guardar snapshot y cerrar.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("recibida señal %s, apagando...", sig)
		if err := s.Save(); err != nil {
			log.Printf("error al guardar snapshot final: %v", err)
		} else if *snapshot != "" {
			log.Printf("snapshot final guardado en %s", *snapshot)
		}
		_ = s.Close()
	}()

	if err := s.Serve(); err != nil {
		log.Fatalf("servidor detenido: %v", err)
	}
	log.Println("apagado limpio")
}
```

- [ ] **Step 2: Verificar que compila y vet pasa**

Run: `go vet ./... && go build ./...`
Expected: sin avisos, build OK.

- [ ] **Step 3: Commit**

```bash
git add cmd/llavero/main.go
git -c user.email="developert@orvixapp.com" -c user.name="cesar" commit -m "feat: apagado ordenado con snapshot final ante SIGINT/SIGTERM"
```

---

### Task 3: Verificación final de la fase

**Files:** ninguno (solo verificación).

- [ ] **Step 1: vet + build + suite completa con -race**

Run: `go vet ./... && go build ./... && go test ./... -race`
Expected: vet sin avisos, build OK, todos los paquetes en verde.

- [ ] **Step 2: Prueba de humo manual del apagado con snapshot final**

Asegurarse de que el puerto 6380 está libre (`ss -ltnp | grep 6380`; matar por PID si hace falta). Luego:
```bash
TMP=$(mktemp -d)
go build -o /tmp/llavero-bin ./cmd/llavero
/tmp/llavero-bin -snapshot "$TMP/dump.llavero" &
SRV=$!
sleep 1
# escribir una clave con un cliente mini-RESP mínimo
printf '*3\n$3\nSET\n$1\nk\n$5\nhola\n' | { exec 3<>/dev/tcp/localhost/6380 2>/dev/null && cat >&3 && head -c 4 <&3; } 2>/dev/null || \
  go run - <<'GOEOF'
package main
import ("bufio";"fmt";"net";"time")
func main(){
  var c net.Conn; var e error
  for i:=0;i<30;i++{ if c,e=net.Dial("tcp","localhost:6380");e==nil{break}; time.Sleep(50*time.Millisecond) }
  if e!=nil{ fmt.Println("no conecta:",e); return }
  defer c.Close()
  fmt.Fprintf(c,"*3\n$3\nSET\n$1\nk\n$5\nhola\n")
  r:=bufio.NewReader(c); line,_:=r.ReadString('\n'); fmt.Printf("SET -> %q\n", line)
}
GOEOF
# mandar SIGTERM: debe guardar el snapshot final
kill -TERM $SRV
sleep 1
echo "--- ¿existe el snapshot? ---"
ls -la "$TMP/dump.llavero" && echo "SNAPSHOT GUARDADO" || echo "FALTA EL SNAPSHOT"
rm -rf "$TMP" /tmp/llavero-bin
```
Expected: tras el `kill -TERM`, el archivo `$TMP/dump.llavero` existe (no vacío) → el apagado guardó el snapshot final. Los logs del servidor muestran "recibida señal terminated, apagando..." y "apagado limpio".

Nota de operación: si el binario no logra enlazar el puerto, matar cualquier servidor previo en 6380 por PID antes de reintentar.

---

## Resultado de la fase

Al terminar la Fase 6, Llavero se apaga de forma ordenada ante SIGINT/SIGTERM y
guarda un snapshot final, eliminando la pérdida de datos al cerrar con los flags
por defecto. La Fase 7 añadirá comandos string/admin (`INCR`, `MSET/MGET`,
`SETNX`, `TYPE`, `KEYS`, `DBSIZE`, `FLUSHALL`).
