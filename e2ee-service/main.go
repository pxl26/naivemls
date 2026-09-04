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

	"github.com/pxl26/naivemls/e2ee-service/internal/e2ee"
)

func main() {
	addr := flag.String("addr", ":8081", "listen address")
	data := flag.String("data", "./data/e2ee.txt", "E2EE state file")
	flag.Parse()

	store, err := e2ee.OpenStore(*data)
	if err != nil {
		log.Fatalf("open E2EE store: %v", err)
	}
	server := &http.Server{
		Addr:              *addr,
		Handler:           e2ee.NewRouter(store),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("e2ee-service listening on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errCh:
		log.Fatal(err)
	case <-stop:
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
