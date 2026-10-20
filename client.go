package setalabs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"
)

const defaultTimeout = 30 * time.Second

var (
	apiUUIDPattern   = regexp.MustCompile(`^[a-z0-9_-]+$`)
	lineBreakPattern = regexp.MustCompile(`[\r\n]`)
	allowedMethods   = map[string]struct{}{
		http.MethodGet:    {},
		http.MethodPost:   {},
		http.MethodPut:    {},
		http.MethodPatch:  {},
		http.MethodDelete: {},
		http.MethodHead:   {},
	}
	reservedHeaders = map[string]struct{}{
		"x-api-key":       {},
		"x-api-secret":    {},
		"x-api-sign":      {},
		"x-api-timestamp": {},
	}
)

// JSONBody is an explicit JSON payload. A nil Value is encoded as JSON null.
// A string Value is sent unchanged.
type JSONBody struct {
	Value any
}

// JSON marks value as the request body.
func JSON(value any) *JSONBody {
	return &JSONBody{Value: value}
}

// Request is one call made through Client.Request.
//
// A zero Timeout uses the client timeout. Query may be a Params value, a map,
// or a raw query string. Set JSON or Body, not both.
type Request struct {
	Query   any
	JSON    *JSONBody
	Body    *string
	Headers any
	Timeout time.Duration
}

// Option configures a Client.
type Option func(*clientOptions)

type clientOptions struct {
	gateway    string
	gatewaySet bool
	timeout    time.Duration
	timeoutSet bool
	transport  Transport
}

// WithGateway sets the gateway root. An empty value uses GatewayURL.
func WithGateway(gateway string) Option {
	return func(options *clientOptions) {
		options.gateway = gateway
		options.gatewaySet = true
	}
}

// WithTimeout sets the default request timeout. It must be greater than zero.
func WithTimeout(timeout time.Duration) Option {
	return func(options *clientOptions) {
		options.timeout = timeout
		options.timeoutSet = true
	}
}

// WithTransport uses a custom transport. The client does not close it.
func WithTransport(transport Transport) Option {
	return func(options *clientOptions) {
		options.transport = transport
	}
}

// Client calls one API published on the Setalabs gateway.
//
// The gateway address, API uuid, key, and secret are separate: a key belongs to
// a single API, and the secret is used only to sign the request. It is never sent.
type Client struct {
	gateway        string
	apiUUID        string
	apiKey         string
	apiSecret      string
	timeout        time.Duration
	transport      Transport
	shareTransport bool
	released       bool
	mu             sync.Mutex
}

// New returns a client for one API. apiSecret may be empty when the key is not signed.
func New(apiUUID, apiKey, apiSecret string, opts ...Option) (*Client, error) {
	options := clientOptions{timeout: defaultTimeout}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	gateway := GatewayURL
	if options.gatewaySet {
		gateway = options.gateway
	}

	var created *HTTPTransport
	transport := options.transport
	if transport == nil {
		created = NewHTTPTransport(nil)
		transport = created
	}

	client := &Client{}
	err := client.configure(apiUUID, apiKey, apiSecret, gateway, options.timeout, transport, created != nil)
	if err != nil {
		if created != nil {
			created.Close()
		}
		return nil, err
	}
	return client, nil
}

// WithAPI returns a client for another API on the same gateway.
// A key is valid only for the API it was issued for.
// Close each client: a shared default transport closes after the last Close.
func (c *Client) WithAPI(apiUUID, apiKey, apiSecret string) (*Client, error) {
	clone := &Client{}
	if err := clone.configure(apiUUID, apiKey, apiSecret, c.gateway, c.timeout, c.transport, c.shareTransport); err != nil {
		return nil, err
	}
	return clone, nil
}

// Get calls an endpoint. query may be nil, Params, a map, or a raw query string.
func (c *Client) Get(ctx context.Context, path string, query any, headers any) (*Response, error) {
	return c.Request(ctx, http.MethodGet, path, Request{Query: query, Headers: headers})
}

// Post sends jsonBody when it is not nil. A nil jsonBody sends no body.
func (c *Client) Post(ctx context.Context, path string, jsonBody any, query any, headers any) (*Response, error) {
	return c.withJSON(ctx, http.MethodPost, path, jsonBody, query, headers)
}

// Put sends jsonBody when it is not nil. A nil jsonBody sends no body.
func (c *Client) Put(ctx context.Context, path string, jsonBody any, query any, headers any) (*Response, error) {
	return c.withJSON(ctx, http.MethodPut, path, jsonBody, query, headers)
}

// Patch sends jsonBody when it is not nil. A nil jsonBody sends no body.
func (c *Client) Patch(ctx context.Context, path string, jsonBody any, query any, headers any) (*Response, error) {
	return c.withJSON(ctx, http.MethodPatch, path, jsonBody, query, headers)
}

// Delete sends jsonBody when it is not nil. A nil jsonBody sends no body.
func (c *Client) Delete(ctx context.Context, path string, jsonBody any, query any, headers any) (*Response, error) {
	return c.withJSON(ctx, http.MethodDelete, path, jsonBody, query, headers)
}

// Request sends one signed call. The path is the endpoint path from the OpenAPI
// document, without the /v1/api/{uuid} prefix.
func (c *Client) Request(ctx context.Context, method, path string, req Request) (*Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	method = strings.ToUpper(method)
	if _, ok := allowedMethods[method]; !ok {
		return nil, sprintfArgument("Unsupported HTTP method \"%s\".", method)
	}

	headers, err := normalizeHeaders(req.Headers)
	if err != nil {
		return nil, err
	}
	queryString, err := buildQuery(req.Query)
	if err != nil {
		return nil, err
	}
	decodedPath, err := c.decodedPath(path)
	if err != nil {
		return nil, err
	}
	pathWithQuery := decodedPath
	if queryString != "" {
		pathWithQuery += "?" + queryString
	}
	body, err := requestBody(req, headers)
	if err != nil {
		return nil, err
	}

	headers["X-Api-Key"] = c.apiKey
	if c.apiSecret != "" {
		timestamp := fmt.Sprintf("%d", time.Now().Unix())
		headers["X-Api-Timestamp"] = timestamp
		headers["X-Api-Sign"] = Sign(c.apiKey, c.apiSecret, timestamp, pathWithQuery)
	}

	rawURL := c.gateway + encodePath(decodedPath)
	if queryString != "" {
		rawURL += "?" + queryString
	}

	timeout := c.timeout
	if req.Timeout != 0 {
		if req.Timeout < 0 {
			return nil, argument("Timeout must be greater than zero.")
		}
		timeout = req.Timeout
	}

	response, err := c.transport.Send(ctx, method, rawURL, headers, body, timeout)
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, &TransportError{Message: "Request failed."}
	}
	if response.StatusCode >= 400 {
		return nil, ErrorFromResponse(response)
	}
	return response, nil
}

// Close releases one use of a shared default transport.
func (c *Client) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.released {
		return
	}
	c.released = true
	if c.shareTransport {
		if releaser, ok := c.transport.(interface{ Release() }); ok {
			releaser.Release()
		}
	}
}

func (c *Client) withJSON(ctx context.Context, method, path string, jsonBody any, query any, headers any) (*Response, error) {
	req := Request{Query: query, Headers: headers}
	if jsonBody != nil {
		req.JSON = JSON(jsonBody)
	}
	return c.Request(ctx, method, path, req)
}

func (c *Client) configure(apiUUID, apiKey, apiSecret, gateway string, timeout time.Duration, transport Transport, shareTransport bool) error {
	normalized, err := normalizeGateway(gateway)
	if err != nil {
		return err
	}
	c.gateway = normalized
	c.apiUUID = strings.ToLower(strings.TrimSpace(apiUUID))
	c.apiKey = strings.TrimSpace(apiKey)
	c.apiSecret = strings.TrimSpace(apiSecret)
	c.timeout = timeout
	c.transport = transport
	c.shareTransport = shareTransport
	c.released = false
	if err := c.assertConfig(); err != nil {
		return err
	}
	if shareTransport {
		if acquirer, ok := transport.(interface{ Acquire() }); ok {
			acquirer.Acquire()
		}
	}
	return nil
}

func (c *Client) assertConfig() error {
	if c.timeout <= 0 {
		return argument("Timeout must be greater than zero.")
	}
	if c.apiUUID == "" || !apiUUIDPattern.MatchString(c.apiUUID) {
		return argument("API uuid must contain only lowercase letters, digits, \"_\" and \"-\".")
	}
	if c.apiKey == "" {
		return argument("API key is required.")
	}
	if err := assertHeaderSafe("API key", c.apiKey); err != nil {
		return err
	}
	return assertHeaderSafe("API secret", c.apiSecret)
}

func (c *Client) decodedPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if strings.Contains(path, "?") || strings.Contains(path, "#") {
		return "", argument("Pass query parameters separately from the path.")
	}
	relative := ""
	if path != "" && path != "/" {
		relative = "/" + strings.TrimLeft(path, "/")
	}
	return APIPrefix + "/" + c.apiUUID + relative, nil
}

func normalizeGateway(gateway string) (string, error) {
	if strings.TrimSpace(gateway) == "" {
		gateway = GatewayURL
	}
	gateway = strings.TrimRight(strings.TrimSpace(gateway), "/")
	parsed, err := url.Parse(gateway)
	if err != nil || !validGatewayScheme(parsed) {
		return "", argument("Gateway address must be an absolute URL, for example https://gw.setalabs.com.")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", argument("Gateway address must be the gateway root, without /v1/api/{uuid}. Pass the API uuid separately.")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", argument("Gateway address must not contain a query or fragment.")
	}
	return gateway, nil
}

func validGatewayScheme(parsed *url.URL) bool {
	if parsed == nil || parsed.Hostname() == "" {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return true
	default:
		return false
	}
}

func encodePath(decodedPath string) string {
	segments := strings.Split(decodedPath, "/")
	for i, segment := range segments {
		if segment != "" {
			segments[i] = escapeRFC3986(segment)
		}
	}
	return strings.Join(segments, "/")
}

func requestBody(req Request, headers map[string]string) (*string, error) {
	if req.JSON != nil && req.Body != nil {
		return nil, argument("Pass either json or body, not both.")
	}
	if req.JSON != nil {
		if !hasHeader(headers, "Content-Type") {
			headers["Content-Type"] = "application/json"
		}
		encoded, err := encodeJSON(req.JSON.Value)
		if err != nil {
			return nil, err
		}
		return &encoded, nil
	}
	if req.Body != nil {
		body := *req.Body
		return &body, nil
	}
	return nil, nil
}

func encodeJSON(value any) (string, error) {
	if text, ok := value.(string); ok {
		return text, nil
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return "", argument("JSON body could not be encoded.")
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

func normalizeHeaders(headers any) (map[string]string, error) {
	normalized := map[string]string{}
	if headers == nil {
		addDefaultHeaders(normalized)
		return normalized, nil
	}

	value := reflect.ValueOf(headers)
	if value.Kind() != reflect.Map || value.Type().Key().Kind() != reflect.String {
		return nil, argument("Headers must be a map.")
	}
	for _, key := range value.MapKeys() {
		name := key.String()
		if name == "" {
			return nil, argument("Header name must be a non-empty string.")
		}
		if _, reserved := reservedHeaders[strings.ToLower(name)]; reserved {
			continue
		}
		text, ok := scalarString(value.MapIndex(key).Interface(), "")
		if !ok {
			return nil, sprintfArgument("Header \"%s\" must be a scalar.", name)
		}
		if err := assertHeaderSafe(fmt.Sprintf("Header \"%s\"", name), text); err != nil {
			return nil, err
		}
		replaceHeader(normalized, name, text)
	}
	addDefaultHeaders(normalized)
	return normalized, nil
}

func addDefaultHeaders(headers map[string]string) {
	if !hasHeader(headers, "Accept") {
		headers["Accept"] = "application/json"
	}
	if !hasHeader(headers, "User-Agent") {
		headers["User-Agent"] = "setalabs-go-sdk/" + Version
	}
}

func replaceHeader(headers map[string]string, name, value string) {
	for existing := range headers {
		if strings.EqualFold(existing, name) {
			delete(headers, existing)
			break
		}
	}
	headers[name] = value
}

func hasHeader(headers map[string]string, name string) bool {
	for existing := range headers {
		if strings.EqualFold(existing, name) {
			return true
		}
	}
	return false
}

func assertHeaderSafe(label, value string) error {
	if lineBreakPattern.MatchString(value) {
		return argument(label + " must not contain line breaks.")
	}
	return nil
}
