package setalabs

import (
	"encoding/json"
	"strings"
)

// Response is the upstream status, headers, and body returned by the gateway.
// Header names are stored in lowercase.
type Response struct {
	StatusCode int
	Headers    map[string][]string
	Body       string
}

// NewResponse normalizes header names to lowercase and copies their values.
func NewResponse(statusCode int, headers map[string][]string, body string) *Response {
	normalized := make(map[string][]string, len(headers))
	for name, values := range headers {
		key := strings.ToLower(name)
		copied := append([]string(nil), values...)
		normalized[key] = append(normalized[key], copied...)
	}
	return &Response{
		StatusCode: statusCode,
		Headers:    normalized,
		Body:       body,
	}
}

// Successful reports whether the status is in the 2xx range.
func (r *Response) Successful() bool {
	return r != nil && r.StatusCode >= 200 && r.StatusCode < 300
}

// Header returns the first value of a header. The name is case-insensitive.
func (r *Response) Header(name string) string {
	if r == nil {
		return ""
	}
	values := r.Headers[strings.ToLower(name)]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// RequestID returns the gateway X-Request-Id header.
func (r *Response) RequestID() string {
	if r == nil {
		return ""
	}
	return r.Header("X-Request-Id")
}

// JSON decodes the response body.
func (r *Response) JSON() (any, error) {
	if r == nil || r.Body == "" {
		return nil, &ResponseError{Message: "Response body is empty."}
	}
	var decoded any
	if err := json.Unmarshal([]byte(r.Body), &decoded); err != nil {
		return nil, &ResponseError{Message: "Response body is not valid JSON."}
	}
	return decoded, nil
}
