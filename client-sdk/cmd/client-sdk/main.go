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

	mls "github.com/pxl26/naivemls/client-sdk"
	"github.com/pxl26/naivemls/client-sdk/internal/httpapi"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	e2eeURL := flag.String("e2ee-url", "http://127.0.0.1:8081", "E2EE service base URL")
	messageURL := flag.String("message-url", "http://127.0.0.1:8082", "message service base URL")
	flag.Parse()

	client := mls.NewClient(*e2eeURL, *messageURL)
	server := &http.Server{
		Addr:              *addr,
		Handler:           httpapi.NewRouter(client, mls.PassthroughCodec{}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("client-sdk listening on %s (development passthrough codec)", server.Addr)
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
