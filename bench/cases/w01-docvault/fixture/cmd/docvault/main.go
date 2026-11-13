// Command docvault serves a document tree over HTTP.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	docvault "example.com/docvault"
)

func main() {
	var (
		listen  = flag.String("listen", "", "address to listen on (overrides DOCVAULT_ADDR)")
		root    = flag.String("root", "", "document root (overrides DOCVAULT_ROOT)")
		secret  = flag.String("token-secret", "", "token secret (overrides DOCVAULT_TOKEN_SECRET)")
		tenants = flag.String("tenants", "", "comma-separated tenant ids (overrides DOCVAULT_TENANTS)")
		verbose = flag.Bool("v", false, "log every request and the wiring")
	)
	flag.Parse()

	opts := docvault.Options{
		Root:        firstNonEmpty(*root, os.Getenv("DOCVAULT_ROOT")),
		Addr:        firstNonEmpty(*listen, os.Getenv("DOCVAULT_ADDR")),
		Tenants:     splitList(firstNonEmpty(*tenants, os.Getenv("DOCVAULT_TENANTS"))),
		TokenSecret: firstNonEmpty(*secret, os.Getenv("DOCVAULT_TOKEN_SECRET")),
	}
	if raw := strings.TrimSpace(os.Getenv("DOCVAULT_PAGE_SIZE")); raw != "" {
		n, err := time.ParseDuration(raw + "s")
		if err == nil {
			opts.PageSize = int(n / time.Second)
		}
	}

	service, err := docvault.Open(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "docvault:", err)
		os.Exit(2)
	}

	var logger *log.Logger
	if *verbose {
		logger = log.New(os.Stderr, "docvault ", log.LstdFlags)
		logger.Printf("root=%s addr=%s routes=%d", service.Root(), service.Addr(), len(service.Routes()))
	}

	server := &http.Server{
		Addr:              service.Addr(),
		Handler:           service.Handler(logger),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		fmt.Fprintln(os.Stderr, "docvault:", err)
		os.Exit(1)
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

func splitList(v string) []string {
	var out []string
	for _, item := range strings.Split(v, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
