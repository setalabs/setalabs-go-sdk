package setalabs

import "fmt"

// Error is the base type for gateway and transport failures.
// Invalid client arguments are ArgumentError and do not implement Error.
type Error interface {
	error
	setalabs()
}

// ArgumentError is returned when a client argument is invalid.
type ArgumentError struct {
	Message string
}

func (e *ArgumentError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func argument(message string) error {
	return &ArgumentError{Message: message}
}

// TransportError means the request did not receive an HTTP response.
type TransportError struct {
	Message string
	Err     error
}

func (e *TransportError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return "Request failed."
}

func (e *TransportError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *TransportError) setalabs() {}

func transportFailure(err error) error {
	message := "Request failed."
	if err != nil && err.Error() != "" {
		message = err.Error()
	}
	return &TransportError{Message: message, Err: err}
}

// ResponseError means the response body could not be decoded.
type ResponseError struct {
	Message string
}

func (e *ResponseError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *ResponseError) setalabs() {}

// APIError is a gateway or upstream response with status 400 or higher.
type APIError struct {
	StatusCode    int
	ErrorCode     string
	Reason        string
	RequestID     string
	MissingFields []string
	RetryAfter    *int
	Response      *Response
	message       string
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	return e.message
}

func (e *APIError) setalabs() {}

// BadRequestError means the API has no base URL, or the gateway rejected the request.
type BadRequestError struct{ *APIError }

func (e *BadRequestError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.APIError
}

// UnauthorizedError means the key, signature, or timestamp is missing or invalid.
type UnauthorizedError struct{ *APIError }

func (e *UnauthorizedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.APIError
}

// ForbiddenError means the key is blocked, expired, without a tariff, or belongs to another API.
type ForbiddenError struct{ *APIError }

func (e *ForbiddenError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.APIError
}

// NotFoundError means the API or endpoint was not found.
type NotFoundError struct{ *APIError }

func (e *NotFoundError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.APIError
}

// ValidationError means required fields are missing. Their names are in MissingFields.
type ValidationError struct{ *APIError }

func (e *ValidationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.APIError
}

// TooManyRequestsError means the gateway refused the call because of a limit. Status is 429.
type TooManyRequestsError struct{ *APIError }

func (e *TooManyRequestsError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.APIError
}

// RateLimitError means the plan RPS limit was exceeded. Wait RetryAfter seconds.
type RateLimitError struct{ *TooManyRequestsError }

func (e *RateLimitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.TooManyRequestsError
}

// UsageLimitError means the call quota for the period is exhausted.
type UsageLimitError struct{ *TooManyRequestsError }

func (e *UsageLimitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.TooManyRequestsError
}

// ServerError means the gateway failed internally.
type ServerError struct{ *APIError }

func (e *ServerError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.APIError
}

// BadGatewayError means the upstream API is unavailable.
type BadGatewayError struct{ *APIError }

func (e *BadGatewayError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.APIError
}

// Error codes returned by the Setalabs gateway.
const (
	ErrorBadRequest         = "bad_request"
	ErrorUnauthorized       = "unauthorized"
	ErrorForbidden          = "forbidden"
	ErrorNotFound           = "not_found"
	ErrorValidation         = "validation_error"
	ErrorBadGateway         = "bad_gateway"
	ErrorInternal           = "internal_error"
	ErrorRateLimited        = "rate_limited"
	ErrorUsageLimitExceeded = "usage_limit_exceeded"
)

func typedAPIError(api *APIError) error {
	switch api.ErrorCode {
	case ErrorBadRequest:
		return &BadRequestError{APIError: api}
	case ErrorUnauthorized:
		return &UnauthorizedError{APIError: api}
	case ErrorForbidden:
		return &ForbiddenError{APIError: api}
	case ErrorNotFound:
		return &NotFoundError{APIError: api}
	case ErrorValidation:
		return &ValidationError{APIError: api}
	case ErrorRateLimited:
		return &RateLimitError{TooManyRequestsError: &TooManyRequestsError{APIError: api}}
	case ErrorUsageLimitExceeded:
		return &UsageLimitError{TooManyRequestsError: &TooManyRequestsError{APIError: api}}
	case ErrorBadGateway:
		return &BadGatewayError{APIError: api}
	case ErrorInternal:
		return &ServerError{APIError: api}
	}
	switch api.StatusCode {
	case 400:
		return &BadRequestError{APIError: api}
	case 401:
		return &UnauthorizedError{APIError: api}
	case 403:
		return &ForbiddenError{APIError: api}
	case 404:
		return &NotFoundError{APIError: api}
	case 422:
		return &ValidationError{APIError: api}
	case 429:
		return &TooManyRequestsError{APIError: api}
	case 500:
		return &ServerError{APIError: api}
	case 502, 503, 504:
		return &BadGatewayError{APIError: api}
	default:
		return api
	}
}

func sprintfArgument(format string, args ...any) error {
	return argument(fmt.Sprintf(format, args...))
}
