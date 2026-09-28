package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBootReadiness(t *testing.T) {
	for _, tc := range []struct {
		path string
		code int
	}{{"/hc", 200}, {"/ready", 503}, {"/manifest", 503}} {
		w := httptest.NewRecorder()
		bootHandler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if w.Code != tc.code {
			t.Fatalf("%s: got %d, want %d", tc.path, w.Code, tc.code)
		}
	}
}

func TestShutdownDrainsActiveRequest(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		_, _ = io.WriteString(w, "published")
	}))
	defer server.Close()
	response := make(chan error, 1)
	go func() {
		r, err := server.Client().Get(server.URL)
		if err == nil {
			defer r.Body.Close()
			var body []byte
			body, err = io.ReadAll(r.Body)
			if err == nil && string(body) != "published" {
				err = errors.New("response was truncated")
			}
		}
		response <- err
	}()
	<-started
	drained := make(chan error, 1)
	go func() { drained <- shutdownServer(server.Config, 5*time.Second) }()
	select {
	case err := <-drained:
		close(release)
		t.Fatalf("shutdown returned before active request completed: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-response; err != nil {
		t.Fatal(err)
	}
	if err := <-drained; err != nil {
		t.Fatal(err)
	}
}

func TestShutdownForcesCloseAfterDeadline(t *testing.T) {
	started, canceled := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(canceled)
	}))
	defer server.Close()
	go func() {
		response, err := server.Client().Get(server.URL)
		if err == nil {
			response.Body.Close()
		}
	}()
	<-started
	if err := shutdownServer(server.Config, 20*time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline, got %v", err)
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("active connection was not closed")
	}
}
