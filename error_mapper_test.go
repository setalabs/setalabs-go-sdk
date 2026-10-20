package setalabs

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestMapsGatewayError(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		want    func(error) bool
		message string
	}{
		{"bad request", 400, gatewayJSON(ErrorBadRequest, "API has no base URL."), isError[*BadRequestError], "API has no base URL."},
		{"missing key", 401, gatewayJSON(ErrorUnauthorized, "Missing API key."), isError[*UnauthorizedError], "Missing API key."},
		{"invalid key", 401, gatewayJSON(ErrorUnauthorized, "Invalid API key."), isError[*UnauthorizedError], "Invalid API key."},
		{"missing signature", 401, gatewayJSON(ErrorUnauthorized, "Missing API signature."), isError[*UnauthorizedError], "Missing API signature."},
		{"missing timestamp", 401, gatewayJSON(ErrorUnauthorized, "Missing API timestamp."), isError[*UnauthorizedError], "Missing API timestamp."},
		{"invalid signature", 401, gatewayJSON(ErrorUnauthorized, "Invalid API signature."), isError[*UnauthorizedError], "Invalid API signature."},
		{"blocked", 403, gatewayJSON(ErrorForbidden, "API key is blocked."), isError[*ForbiddenError], "API key is blocked."},
		{"expired", 403, gatewayJSON(ErrorForbidden, "API key subscription has expired."), isError[*ForbiddenError], "API key subscription has expired."},
		{"no tariff", 403, gatewayJSON(ErrorForbidden, "API key has no active tariff."), isError[*ForbiddenError], "API key has no active tariff."},
		{"wrong api", 403, gatewayJSON(ErrorForbidden, "API key is not valid for this API."), isError[*ForbiddenError], "API key is not valid for this API."},
		{"api not found", 404, gatewayJSON(ErrorNotFound, "API not found."), isError[*NotFoundError], "API not found."},
		{"endpoint not found", 404, gatewayJSON(ErrorNotFound, "API endpoint not found."), isError[*NotFoundError], "API endpoint not found."},
		{"not found without reason", 404, gatewayJSON(ErrorNotFound, ""), isError[*NotFoundError], ErrorNotFound},
		{"bad gateway", 502, gatewayJSON(ErrorBadGateway, "Upstream API is unavailable."), isError[*BadGatewayError], "Upstream API is unavailable."},
		{"empty reason", 500, `{"error":"internal_error","reason":""}`, isError[*ServerError], ErrorInternal},
		{"status fallback", 503, "nope", isError[*BadGatewayError], "HTTP 503"},
		{"too many requests", 429, "nope", isError[*TooManyRequestsError], "HTTP 429"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ErrorFromResponse(textResponse(test.status, map[string]string{"X-Request-Id": "abc123"}, test.body))
			if !test.want(err) {
				t.Fatalf("error type = %T", err)
			}
			if err.Error() != test.message {
				t.Fatalf("message = %q, want %q", err.Error(), test.message)
			}
			var api *APIError
			if !errors.As(err, &api) {
				t.Fatal("expected APIError")
			}
			if api.StatusCode != test.status || api.RequestID != "abc123" {
				t.Fatalf("status=%d request=%s", api.StatusCode, api.RequestID)
			}
		})
	}
}

func TestValidationErrorIncludesMissingFields(t *testing.T) {
	body, err := json.Marshal(map[string]any{
		"error":          ErrorValidation,
		"reason":         "Required fields are missing.",
		"missing_fields": []string{"page", "limit"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := ErrorFromResponse(textResponse(422, nil, string(body)))
	var validation *ValidationError
	if !errors.As(got, &validation) {
		t.Fatalf("error type = %T", got)
	}
	if validation.Error() != "Required fields are missing. Missing: page, limit." {
		t.Fatalf("message = %q", validation.Error())
	}
	if len(validation.MissingFields) != 2 || validation.MissingFields[0] != "page" || validation.MissingFields[1] != "limit" {
		t.Fatalf("missing fields = %#v", validation.MissingFields)
	}
}

func TestRateLimitReadsRetryAfter(t *testing.T) {
	body, err := json.Marshal(map[string]any{
		"error":       ErrorRateLimited,
		"retry_after": 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := ErrorFromResponse(textResponse(429, map[string]string{
		"Retry-After":       "3",
		"X-RateLimit-Limit": "10",
	}, string(body)))
	var rate *RateLimitError
	if !errors.As(got, &rate) {
		t.Fatalf("error type = %T", got)
	}
	var tooMany *TooManyRequestsError
	if !errors.As(got, &tooMany) {
		t.Fatal("rate limit should also be a too-many-requests error")
	}
	if rate.RetryAfter == nil || *rate.RetryAfter != 3 {
		t.Fatalf("retry after = %#v", rate.RetryAfter)
	}
}

func TestUsageLimit(t *testing.T) {
	got := ErrorFromResponse(textResponse(429, nil, gatewayJSON(ErrorUsageLimitExceeded, "Request limit exceeded.")))
	var usage *UsageLimitError
	if !errors.As(got, &usage) {
		t.Fatalf("error type = %T", got)
	}
	if usage.Error() != "Request limit exceeded." {
		t.Fatalf("message = %q", usage.Error())
	}
	if usage.RetryAfter != nil {
		t.Fatalf("retry after = %#v", usage.RetryAfter)
	}
}

func TestPlainUpstreamErrorUsesStatus(t *testing.T) {
	got := ErrorFromResponse(textResponse(418, nil, "nope"))
	var api *APIError
	if !errors.As(got, &api) {
		t.Fatalf("error type = %T", got)
	}
	var badRequest *BadRequestError
	if errors.As(got, &badRequest) {
		t.Fatal("plain upstream error should stay an APIError")
	}
	if got.Error() != "HTTP 418" || api.ErrorCode != "" {
		t.Fatalf("error = %#v", api)
	}
}

func gatewayJSON(code, reason string) string {
	payload := map[string]string{"error": code}
	if reason != "" {
		payload["reason"] = reason
	}
	body, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return string(body)
}

func textResponse(status int, headers map[string]string, body string) *Response {
	converted := make(map[string][]string, len(headers))
	for name, value := range headers {
		converted[name] = []string{value}
	}
	return NewResponse(status, converted, body)
}

func isError[T any](err error) bool {
	var target T
	return errors.As(err, &target)
}
