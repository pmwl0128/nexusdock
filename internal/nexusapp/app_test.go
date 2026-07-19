package nexusapp

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestServeHTTPReturnsListenFailure(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()

	server := &http.Server{Addr: occupied.Addr().String(), Handler: http.NotFoundHandler()}
	err = serveHTTP(context.Background(), server)
	if err == nil || !strings.Contains(err.Error(), "listen on") {
		t.Fatalf("listen failure=%v", err)
	}
}

func TestServeHTTPShutsDownOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	server := &http.Server{Addr: "127.0.0.1:0", Handler: http.NotFoundHandler()}
	done := make(chan error, 1)
	go func() { done <- serveHTTP(ctx, server) }()
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("graceful shutdown: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("HTTP server did not stop after context cancellation")
	}
}

func TestMainReturnsFailureForInvalidAdminCommand(t *testing.T) {
	if code := Main([]string{"nexusdock", "admin", "unknown"}); code != 1 {
		t.Fatalf("exit code=%d want=1", code)
	}
}
