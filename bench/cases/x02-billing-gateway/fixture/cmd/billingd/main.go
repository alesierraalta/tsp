// Command billingd is the billing gateway's process: it loads the configuration from the environment,
// opens the service and serves it until it is asked to stop.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	billing "example.com/billing"
	"example.com/billing/internal/config"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "billingd:", err)
		os.Exit(1)
	}
}

// run loads the configuration, opens the service and serves it.
func run() error {
	demo := flag.Bool("demo", false, "load the demonstration dataset")
	flag.Parse()

	cfg, err := config.Load(config.OS())
	if err != nil {
		return err
	}
	service, err := billing.Open(billing.Options{
		Addr:           cfg.Addr,
		Tenants:        cfg.Tenants,
		TokenSecret:    cfg.TokenSecret,
		Currency:       cfg.Currency,
		CycleMonths:    cfg.CycleMonths,
		GraceDays:      cfg.GraceDays,
		RetentionDays:  cfg.RetentionDays,
		PageSize:       cfg.MaxPageSize,
		MaxInvoiceRows: cfg.MaxInvoiceRows,
		MaxEvents:      cfg.MaxEvents,
		MaxBodyBytes:   cfg.MaxBodyBytes,
		SessionTTL:     cfg.SessionTTL,
		TaxBasis:       cfg.TaxRateBasis,
		AdminSubjects:  cfg.AdminSubjects,
		Seed:           *demo,
	})
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           service.Handler(nil),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		log.Println("billingd: stopping")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Println("billingd: shutdown:", err)
		}
	}()
	log.Printf("billingd: listening on %s, tenants %v", cfg.Addr, cfg.Tenants)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
