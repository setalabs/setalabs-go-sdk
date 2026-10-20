package setalabs

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"
)

const (
	testUUID   = "05141812-7772-403e-9ca6-5e8e8bf22730"
	testKey    = "key-1"
	testSecret = "secret-1"
)

func TestSignedGetUsesGatewayPathAndKeepsQueryOrder(t *testing.T) {
	transport := &recordingTransport{response: textResponse(200, map[string]string{
		"X-Request-Id": "req-1",
		"Content-Type": "application/json",
	}, `{"ok":true}`)}
	client := testClient(t, transport)

	response, err := client.Get(context.Background(), "/catalog", Params{
		{Name: "b", Value: "2"},
		{Name: "a", Value: "1"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !response.Successful() {
		t.Fatal("expected a successful response")
	}
	decoded, err := response.JSON()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, map[string]any{"ok": true}) {
		t.Fatalf("json = %#v", decoded)
	}
	if response.RequestID() != "req-1" {
		t.Fatalf("request id = %s", response.RequestID())
	}

	call := transport.calls[0]
	pathWithQuery := "/v1/api/" + testUUID + "/catalog?b=2&a=1"
	if call.method != http.MethodGet {
		t.Fatalf("method = %s", call.method)
	}
	if call.rawURL != GatewayURL+pathWithQuery {
		t.Fatalf("url = %s", call.rawURL)
	}
	if call.headers["X-Api-Key"] != testKey {
		t.Fatalf("key = %s", call.headers["X-Api-Key"])
	}
	if _, ok := call.headers["X-Api-Secret"]; ok {
		t.Fatal("secret was sent")
	}
	if call.headers["X-Api-Sign"] != Sign(testKey, testSecret, call.headers["X-Api-Timestamp"], pathWithQuery) {
		t.Fatal("signature does not match the gateway path")
	}
	if call.headers["Accept"] != "application/json" {
		t.Fatalf("accept = %s", call.headers["Accept"])
	}
	if call.headers["User-Agent"] != "setalabs-go-sdk/"+Version {
		t.Fatalf("user agent = %s", call.headers["User-Agent"])
	}
	if call.timeout != 30*time.Second {
		t.Fatalf("timeout = %s", call.timeout)
	}
}

func TestSignatureUsesDecodedPathAndEncodedQuery(t *testing.T) {
	transport := &recordingTransport{response: textResponse(200, nil, `{"items":[]}`)}
	client := testClient(t, transport)

	if _, err := client.Get(context.Background(), "/product/a b", Params{{Name: "q", Value: "x y"}}, nil); err != nil {
		t.Fatal(err)
	}

	call := transport.calls[0]
	signed := "/v1/api/" + testUUID + "/product/a b?q=x%20y"
	wire := GatewayURL + "/v1/api/" + testUUID + "/product/a%20b?q=x%20y"
	if call.rawURL != wire {
		t.Fatalf("url = %s", call.rawURL)
	}
	if call.headers["X-Api-Sign"] != Sign(testKey, testSecret, call.headers["X-Api-Timestamp"], signed) {
		t.Fatal("signature was computed from the encoded path")
	}
}

func TestKeyWithoutSecretIsNotSigned(t *testing.T) {
	transport := &recordingTransport{response: textResponse(200, nil, "{}")}
	client, err := New(testUUID, testKey, "", WithGateway(GatewayURL), WithTimeout(30*time.Second), WithTransport(transport))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := client.Get(context.Background(), "/langs", nil, nil); err != nil {
		t.Fatal(err)
	}

	headers := transport.calls[0].headers
	if headers["X-Api-Key"] != testKey {
		t.Fatalf("key = %s", headers["X-Api-Key"])
	}
	for _, name := range []string{"X-Api-Sign", "X-Api-Timestamp", "X-Api-Secret"} {
		if _, ok := headers[name]; ok {
			t.Fatalf("%s was sent", name)
		}
	}
	if transport.calls[0].rawURL != GatewayURL+"/v1/api/"+testUUID+"/langs" {
		t.Fatalf("url = %s", transport.calls[0].rawURL)
	}
}

func TestPostSendsJSONBody(t *testing.T) {
	transport := &recordingTransport{response: textResponse(201, nil, `{"id":"1"}`)}
	client := testClient(t, transport)

	response, err := client.Post(context.Background(), "/orders", map[string]any{"sku": "a/b"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 201 {
		t.Fatalf("status = %d", response.StatusCode)
	}
	if transport.calls[0].body == nil || *transport.calls[0].body != `{"sku":"a/b"}` {
		t.Fatalf("body = %#v", transport.calls[0].body)
	}
	if transport.calls[0].headers["Content-Type"] != "application/json" {
		t.Fatalf("content type = %s", transport.calls[0].headers["Content-Type"])
	}
	if transport.calls[0].method != http.MethodPost {
		t.Fatalf("method = %s", transport.calls[0].method)
	}
}

func TestGatewayErrorIsReturned(t *testing.T) {
	transport := &recordingTransport{response: textResponse(401, map[string]string{
		"X-Request-Id": "req-9",
	}, `{"error":"unauthorized","reason":"Invalid API signature."}`)}
	client := testClient(t, transport)

	_, err := client.Get(context.Background(), "/catalog", nil, nil)
	var unauthorized *UnauthorizedError
	if !errors.As(err, &unauthorized) {
		t.Fatalf("error type = %T", err)
	}
	if unauthorized.StatusCode != 401 || unauthorized.Error() != "Invalid API signature." || unauthorized.RequestID != "req-9" {
		t.Fatalf("error = %#v", unauthorized.APIError)
	}
}

func TestReservedSecretHeaderIsDropped(t *testing.T) {
	transport := &recordingTransport{response: textResponse(200, nil, "{}")}
	client := testClient(t, transport)

	_, err := client.Get(context.Background(), "/catalog", nil, map[string]any{
		"X-Api-Secret": "do-not-send",
		"X-Trace":      "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := transport.calls[0].headers["X-Api-Secret"]; ok {
		t.Fatal("secret header was sent")
	}
	if transport.calls[0].headers["X-Trace"] != "1" {
		t.Fatalf("trace = %s", transport.calls[0].headers["X-Trace"])
	}
}

func TestWithAPITargetsAnotherUUID(t *testing.T) {
	transport := &recordingTransport{response: textResponse(200, nil, "{}")}
	client, err := testClient(t, transport).WithAPI("another-api", "other-key", "other-secret")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := client.Get(context.Background(), "/health", nil, nil); err != nil {
		t.Fatal(err)
	}

	call := transport.calls[0]
	if call.rawURL != GatewayURL+"/v1/api/another-api/health" {
		t.Fatalf("url = %s", call.rawURL)
	}
	if call.headers["X-Api-Key"] != "other-key" {
		t.Fatalf("key = %s", call.headers["X-Api-Key"])
	}
	const path = "/v1/api/another-api/health"
	if call.headers["X-Api-Sign"] != Sign("other-key", "other-secret", call.headers["X-Api-Timestamp"], path) {
		t.Fatal("signature does not match the other API")
	}
}

func TestGatewayDefaultsToProduction(t *testing.T) {
	transport := &recordingTransport{response: textResponse(200, nil, "{}")}
	client, err := New(testUUID, testKey, testSecret, WithTimeout(30*time.Second), WithTransport(transport))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := client.Get(context.Background(), "/catalog", nil, nil); err != nil {
		t.Fatal(err)
	}
	if transport.calls[0].rawURL != GatewayURL+"/v1/api/"+testUUID+"/catalog" {
		t.Fatalf("url = %s", transport.calls[0].rawURL)
	}
}

func TestGatewayCanBeReplaced(t *testing.T) {
	transport := &recordingTransport{response: textResponse(200, nil, "{}")}
	client, err := New(testUUID, testKey, testSecret, WithGateway("https://gw.example.test/"), WithTimeout(30*time.Second), WithTransport(transport))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := client.Get(context.Background(), "/catalog", nil, nil); err != nil {
		t.Fatal(err)
	}
	if transport.calls[0].rawURL != "https://gw.example.test/v1/api/"+testUUID+"/catalog" {
		t.Fatalf("url = %s", transport.calls[0].rawURL)
	}
}

func TestGatewayMustBeTheRootURL(t *testing.T) {
	_, err := New(testUUID, testKey, testSecret, WithGateway(GatewayURL+"/v1/api/"+testUUID))
	var invalid *ArgumentError
	if !errors.As(err, &invalid) {
		t.Fatalf("error type = %T", err)
	}
}

func TestQueryInPathIsRejected(t *testing.T) {
	client := testClient(t, &recordingTransport{response: textResponse(200, nil, "{}")})
	_, err := client.Get(context.Background(), "/catalog?language=en", nil, nil)
	var invalid *ArgumentError
	if !errors.As(err, &invalid) {
		t.Fatalf("error type = %T", err)
	}
}

func TestRequestSendsJSONQueryAndTimeout(t *testing.T) {
	transport := &recordingTransport{response: textResponse(200, nil, "{}")}
	client := testClient(t, transport)
	body := "raw"

	_, err := client.Request(context.Background(), "put", "/orders/1", Request{
		JSON:    JSON(map[string]any{"status": "paid"}),
		Query:   Params{{Name: "language", Value: "en"}},
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	call := transport.calls[0]
	if call.method != http.MethodPut {
		t.Fatalf("method = %s", call.method)
	}
	if call.body == nil || *call.body != `{"status":"paid"}` {
		t.Fatalf("body = %#v", call.body)
	}
	if call.timeout != 10*time.Second {
		t.Fatalf("timeout = %s", call.timeout)
	}
	signed := "/v1/api/" + testUUID + "/orders/1?language=en"
	if call.rawURL != GatewayURL+signed {
		t.Fatalf("url = %s", call.rawURL)
	}
	if call.headers["X-Api-Sign"] != Sign(testKey, testSecret, call.headers["X-Api-Timestamp"], signed) {
		t.Fatal("signature does not include the query")
	}

	_, err = client.Request(context.Background(), http.MethodPost, "/orders", Request{
		JSON: JSON(map[string]any{"sku": "A1"}),
		Body: &body,
	})
	var invalid *ArgumentError
	if !errors.As(err, &invalid) {
		t.Fatalf("error type = %T", err)
	}
}

func testClient(t *testing.T, transport Transport) *Client {
	t.Helper()
	client, err := New(testUUID, testKey, testSecret, WithGateway(GatewayURL), WithTimeout(30*time.Second), WithTransport(transport))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return client
}

type recordingTransport struct {
	response *Response
	calls    []recordedCall
}

type recordedCall struct {
	method  string
	rawURL  string
	headers map[string]string
	body    *string
	timeout time.Duration
}

func (r *recordingTransport) Send(ctx context.Context, method, rawURL string, headers map[string]string, body *string, timeout time.Duration) (*Response, error) {
	copied := make(map[string]string, len(headers))
	for name, value := range headers {
		copied[name] = value
	}
	var copiedBody *string
	if body != nil {
		value := *body
		copiedBody = &value
	}
	r.calls = append(r.calls, recordedCall{
		method:  method,
		rawURL:  rawURL,
		headers: copied,
		body:    copiedBody,
		timeout: timeout,
	})
	return r.response, nil
}
