// Package diagnostics exposes optional process diagnostics on a private listener.
package diagnostics

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"strings"
	"time"
)

// StartPprofFromEnv starts a dedicated pprof server when addrEnv is set.
func StartPprofFromEnv(addrEnv, allowNonLoopbackEnv string) (*http.Server, error) {
	addr := strings.TrimSpace(os.Getenv(addrEnv))
	if addr == "" {
		return nil, nil
	}
	allowNonLoopback := strings.EqualFold(strings.TrimSpace(os.Getenv(allowNonLoopbackEnv)), "true")
	if !allowNonLoopback {
		if err := requireLoopback(addr); err != nil {
			return nil, fmt.Errorf("%s: %w", addrEnv, err)
		}
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 2 * time.Second, IdleTimeout: 15 * time.Second}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("pprof server stopped: %v", err)
		}
	}()
	log.Printf("pprof diagnostics listening on %s", listener.Addr())
	return server, nil
}

func requireLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid listen address %q: %w", addr, err)
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("listen address %q must use a loopback host", addr)
	}
	return nil
}
