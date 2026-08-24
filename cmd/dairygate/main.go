// Command dairygate runs the single-node DairyGate inspection console: a JSON
// API and the embedded browser console backed by a persistent SQLite WAL
// store. Restart recovery is automatic: on startup the store reopens the
// database and every read rebuilds state from the durable tables.
package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dairygate/raw-milk-tank-intake-inspection/api"
	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/service"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
	"github.com/dairygate/raw-milk-tank-intake-inspection/webembed"
)

func main() {
	cfg := parseConfig()
	run(cfg)
}

func run(cfg *config) {
	st, err := store.OpenSQLite(cfg.dbPath, catalog.NewFixedCatalog())
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	svc := service.NewService(st, service.SystemClock{})
	handler := api.NewHandler(svc)

	mux := http.NewServeMux()
	mux.Handle("/api/", handler)
	mux.Handle("/", webembed.Handler())

	root := api.RequestID(api.Logging(api.Recover(mux)))

	// Bind explicitly so that an ephemeral port (:0) can be reported back to
	// callers (e.g. the smoke test) and so bind failures surface before any
	// goroutine is spawned.
	ln, err := net.Listen("tcp", cfg.addr)
	if err != nil {
		log.Fatalf("listen %s: %v", cfg.addr, err)
	}
	actualAddr := ln.Addr().String()

	srv := &http.Server{
		Handler:           root,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("dairygate listening on %s (db=%s)", actualAddr, cfg.dbPath)
		// A stable, greppable marker used by scripts to discover the port.
		log.Printf("LISTEN_ADDR=%s", actualAddr)
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Fatalf("serve: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
