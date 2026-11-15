// Command ledgerclosed runs the billing close service.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"example.com/ledgerclose"
	"example.com/ledgerclose/internal/config"
)

func main() {
	logger := log.New(os.Stderr, "ledgerclosed ", log.LstdFlags)
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		logger.Fatalf("configuration: %v", err)
	}
	service, err := ledgerclose.Open(cfg)
	if err != nil {
		logger.Fatalf("open: %v", err)
	}
	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           service.Handler(logger),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}

	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
		<-signals
		logger.Print("draining")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			logger.Printf("shutdown: %v", err)
		}
	}()

	logger.Printf("listening on %s with root %s", cfg.Addr, service.Root())
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatalf("listen: %v", err)
	}
	<-stopped
	logger.Print("stopped")
}
