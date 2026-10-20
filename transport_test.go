package setalabs

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPTransportReturnsStatusHeadersAndBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/api/x/catalog" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("accept = %s", r.Header.Get("Accept"))
		}
		w.Header().Set("X-Request-Id", "req-1")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	transport := NewHTTPTransport(nil)
	defer transport.Close()
	response, err := transport.Send(context.Background(), http.MethodGet, server.URL+"/v1/api/x/catalog", map[string]string{
		"Accept": "application/json",
	}, nil, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 || response.RequestID() != "req-1" {
		t.Fatalf("response = %#v", response)
	}
	decoded, err := response.JSON()
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := decoded.(map[string]any)["ok"].(bool); !ok {
		t.Fatalf("json = %#v", decoded)
	}
}

func TestHTTPTransportReturnsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized","reason":"Invalid API key."}`))
	}))
	defer server.Close()

	transport := NewHTTPTransport(nil)
	defer transport.Close()
	response, err := transport.Send(context.Background(), http.MethodGet, server.URL+"/v1/api/x/catalog", nil, nil, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 401 {
		t.Fatalf("status = %d", response.StatusCode)
	}
	decoded, err := response.JSON()
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := decoded.(map[string]any)
	if payload["reason"] != "Invalid API key." {
		t.Fatalf("json = %#v", decoded)
	}
}

func TestHTTPTransportDoesNotFollowRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/other", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	transport := NewHTTPTransport(nil)
	defer transport.Close()
	response, err := transport.Send(context.Background(), http.MethodGet, server.URL+"/start", nil, nil, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusFound {
		t.Fatalf("status = %d", response.StatusCode)
	}
}

func TestHTTPTransportConnectionFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	transport := NewHTTPTransport(nil)
	defer transport.Close()
	_, err = transport.Send(context.Background(), http.MethodGet, "http://"+address+"/v1/api/x/catalog", nil, nil, 2*time.Second)
	var transportErr *TransportError
	if !errors.As(err, &transportErr) {
		t.Fatalf("error type = %T", err)
	}
	if transportErr.Error() == "" {
		t.Fatal("empty transport error")
	}
}

func TestHTTPTransportCanceledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	transport := NewHTTPTransport(nil)
	defer transport.Close()
	_, err := transport.Send(ctx, http.MethodGet, server.URL, nil, nil, 5*time.Second)
	var transportErr *TransportError
	if !errors.As(err, &transportErr) {
		t.Fatalf("error type = %T", err)
	}
}

func TestHTTPTransportLeavesCallerClientOpen(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := server.Client()
	transport := NewHTTPTransport(client)
	transport.Close()

	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload["ok"] != true {
		t.Fatalf("json = %#v", payload)
	}
}
