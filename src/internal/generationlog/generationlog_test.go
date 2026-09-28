package generationlog

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

var fastTiming = Timing{Window: 200 * time.Millisecond, PollInterval: 10 * time.Millisecond}

func TestClientResolveModel_whenLogAppearsAfterPolling_thenReturnsModel(t *testing.T) {
	// Arrange
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte(`{"data":{"model":"m/free"}}`))
	}))
	defer server.Close()
	client := &Client{URL: server.URL, APIKey: "k", HTTP: server.Client(), Timing: fastTiming}

	// Act
	got := client.ResolveModel("gen-1")

	// Assert
	if got != "m/free" {
		t.Fatalf("got %q, want m/free", got)
	}
}

func TestClientResolveModel_whenLogNeverAppears_thenPanics(t *testing.T) {
	// Arrange
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	client := &Client{URL: server.URL, APIKey: "k", HTTP: server.Client(), Timing: fastTiming}

	// Assert
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic")
		}
	}()

	// Act
	client.ResolveModel("gen-1")
}

func TestClientResolveModel_whenUnauthorized_thenPanics(t *testing.T) {
	// Arrange
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	client := &Client{URL: server.URL, APIKey: "k", HTTP: server.Client(), Timing: fastTiming}

	// Assert
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic")
		}
	}()

	// Act
	client.ResolveModel("gen-1")
}
