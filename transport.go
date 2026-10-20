package setalabs

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Transport performs one HTTP call and returns the raw response.
// A non-2xx status is still a successful transport result. The client maps it.
type Transport interface {
	Send(ctx context.Context, method, rawURL string, headers map[string]string, body *string, timeout time.Duration) (*Response, error)
}

// HTTPTransport is the default HTTP implementation, backed by net/http.
type HTTPTransport struct {
	client *http.Client
	owns   bool
	mu     sync.Mutex
	users  int
}

// NewHTTPTransport uses client, or a client that does not follow redirects when client is nil.
func NewHTTPTransport(client *http.Client) *HTTPTransport {
	owns := client == nil
	if owns {
		client = defaultHTTPClient()
	}
	return &HTTPTransport{client: client, owns: owns}
}

// Acquire records one client using a shared transport.
func (t *HTTPTransport) Acquire() {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.users++
	t.mu.Unlock()
}

// Release drops one user. The owned connection pool closes when the last user leaves.
func (t *HTTPTransport) Release() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.users == 0 {
		return
	}
	t.users--
	if t.users == 0 {
		t.closeLocked()
	}
}

// Close closes the owned connection pool.
func (t *HTTPTransport) Close() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closeLocked()
}

func (t *HTTPTransport) closeLocked() {
	if !t.owns {
		return
	}
	t.owns = false
	t.client.CloseIdleConnections()
}

// Send performs the HTTP request. Redirects are not followed when this transport owns the client.
func (t *HTTPTransport) Send(ctx context.Context, method, rawURL string, headers map[string]string, body *string, timeout time.Duration) (*Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	var reader io.Reader
	if body != nil {
		reader = strings.NewReader(*body)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return nil, transportFailure(err)
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, transportFailure(err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, transportFailure(err)
	}
	return NewResponse(resp.StatusCode, resp.Header, string(payload)), nil
}

func defaultHTTPClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 0,
		},
	}
}
