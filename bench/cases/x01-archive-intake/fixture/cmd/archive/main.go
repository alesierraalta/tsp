// Command archive runs the intake service: it loads the configuration from the environment, opens the
// archive, and serves the HTTP surface until it is asked to stop.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	archive "example.com/archive"
	"example.com/archive/internal/config"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "archive:", err)
		os.Exit(1)
	}
}

// run is main's body, so the failure path is testable and the exit code is written once.
func run(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help", "help":
			usage()
			return nil
		default:
			return fmt.Errorf("unknown argument %q", args[0])
		}
	}
	cfg, err := config.Load(os.Environ())
	if err != nil {
		return err
	}
	secret, err := config.RequireEnv(os.Environ(), "ARCHIVE_TOKEN_SECRET")
	if err != nil {
		return err
	}
	service, err := archive.Open(archive.Options{
		Root:          cfg.Root,
		Addr:          cfg.Addr,
		Tenants:       cfg.Tenants,
		PageSize:      cfg.PageSize,
		MaxUpload:     cfg.MaxUpload,
		MaxEntries:    cfg.MaxEntries,
		MaxPages:      cfg.MaxPages,
		TokenSecret:   secret,
		SessionTTL:    cfg.SessionTTL,
		RetentionDays: cfg.RetentionDays,
		Zone:          cfg.Zone,
	})
	if err != nil {
		return err
	}
	logger := log.New(os.Stderr, "archive ", log.LstdFlags|log.LUTC)
	server := &http.Server{
		Addr:              service.Config().Addr,
		Handler:           service.Handler(logger),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	logger.Printf("serving %s root=%s tenants=%s", server.Addr, service.Root(), service.Config().TenantList())
	errc := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
			return
		}
		errc <- nil
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errc:
		return err
	case <-stop:
		logger.Printf("stopping")
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return server.Shutdown(ctx)
	}
}

// usage prints the block an operator reads when they ask how to run the service.
func usage() {
	fmt.Fprint(os.Stderr, `archive - the intake service

  archive            serve the HTTP surface using the environment

Environment:
  ARCHIVE_ROOT            absolute directory the archive owns (required)
  ARCHIVE_TOKEN_SECRET    token signing secret, at least 16 bytes (required)
  ARCHIVE_ADDR            listen address, default 127.0.0.1:8080
  ARCHIVE_TENANTS         comma-separated tenant ids, default acme
  ARCHIVE_PAGE_SIZE       default listing size, default 25
  ARCHIVE_MAX_UPLOAD      largest object, default 32Mi
  ARCHIVE_MAX_ENTRIES     largest manifest entry count, default 500
  ARCHIVE_MAX_PAGES       largest declared page count, default 2000
  ARCHIVE_MAX_EXPORT_ROWS largest export, default 10000
  ARCHIVE_SESSION_TTL     token lifetime, default 1h
  ARCHIVE_RETENTION_DAYS  retention window in UTC days, default 30
  ARCHIVE_ZONE            reporting zone, default UTC
`)
}
