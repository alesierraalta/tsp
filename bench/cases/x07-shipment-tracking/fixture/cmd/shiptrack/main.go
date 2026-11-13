// Command shiptrack runs a shipment-tracking deployment: it loads the configuration from the process
// environment, opens the service and serves it until it is asked to stop.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"example.com/shiptrack"
	"example.com/shiptrack/internal/config"
)

func main() {
	logger := log.New(os.Stderr, "shiptrack ", log.LstdFlags|log.LUTC)
	check := flag.Bool("check", false, "validate the configuration and exit")
	flag.Parse()

	env := config.FromEnviron()
	cfg, err := config.Load(env)
	if err != nil {
		logger.Fatalf("configuration: %v", err)
	}
	if *check {
		for _, line := range cfg.Variables() {
			logger.Print(line)
		}
		return
	}
	service, err := shiptrack.Open(cfg)
	if err != nil {
		logger.Fatalf("open: %v", err)
	}
	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           service.Handler(logger),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			logger.Printf("shutdown: %v", err)
		}
	}()
	logger.Printf("listening on %s, root %s, tenants %s", cfg.Addr, service.Root(), cfg.TenantsText())
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatalf("serve: %v", err)
	}
	logger.Print("stopped")
}
