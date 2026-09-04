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

	messageapi "github.com/pxl26/naivemls/message-service/internal/message"
)

func main() {
	addr := flag.String("addr", ":8082", "listen address")
	data := flag.String("data", "./data/messages.txt", "message state file")
	e2eeURL := flag.String("e2ee-url", "http://127.0.0.1:8081", "E2EE service base URL")
	flag.Parse()

	store, err := messageapi.OpenStore(*data)
	if err != nil {
		log.Fatalf("open message store: %v", err)
	}
	checker := messageapi.NewHTTPEpochChecker(*e2eeURL, 5*time.Second)
	server := &http.Server{
		Addr:              *addr,
		Handler:           messageapi.NewRouter(store, checker),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("message-service listening on %s", server.Addr)
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
