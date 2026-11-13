// Command insightd serves the analytics API.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"example.com/insight"
)

func main() {
	var (
		listen = flag.String("listen", ":8080", "address to listen on")
		seed   = flag.Bool("seed", true, "load the sample rows on start")
	)
	flag.Parse()

	secret := os.Getenv("INSIGHT_TOKEN_SECRET")
	if secret == "" {
		fmt.Fprintln(os.Stderr, "insightd: INSIGHT_TOKEN_SECRET is required")
		os.Exit(2)
	}
	service, err := insight.Open(insight.Options{TokenSecret: secret, Seed: *seed})
	if err != nil {
		fmt.Fprintln(os.Stderr, "insightd:", err)
		os.Exit(2)
	}

	server := &http.Server{
		Addr:              *listen,
		Handler:           service.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("insightd listening on %s with %d reports", *listen, len(service.ReportNames()))
	if err := server.ListenAndServe(); err != nil {
		fmt.Fprintln(os.Stderr, "insightd:", err)
		os.Exit(1)
	}
}
