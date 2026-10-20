package setalabs

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// ErrorFromResponse maps a gateway error response to a typed API error.
func ErrorFromResponse(response *Response) error {
	payload := errorPayload(response)
	code := ""
	reason := ""
	var missing []string
	if payload != nil {
		if value, ok := payload["error"]; ok && value != nil {
			code = asText(value)
		}
		if value, ok := payload["reason"]; ok && value != nil {
			reason = asText(value)
		}
		missing = missingFields(payload["missing_fields"])
	}
	if missing == nil {
		missing = []string{}
	}

	message := fmt.Sprintf("HTTP %d", response.StatusCode)
	if reason != "" {
		message = reason
	} else if code != "" {
		message = code
	}
	if len(missing) > 0 {
		message += " Missing: " + strings.Join(missing, ", ") + "."
	}

	api := &APIError{
		StatusCode:    response.StatusCode,
		ErrorCode:     code,
		Reason:        reason,
		RequestID:     response.RequestID(),
		MissingFields: missing,
		RetryAfter:    retryAfter(response, payload),
		Response:      response,
		message:       message,
	}
	return typedAPIError(api)
}

func errorPayload(response *Response) map[string]any {
	if response == nil || response.Body == "" {
		return nil
	}
	var decoded any
	if err := json.Unmarshal([]byte(response.Body), &decoded); err != nil {
		return nil
	}
	payload, ok := decoded.(map[string]any)
	if !ok {
		return nil
	}
	return payload
}

func missingFields(value any) []string {
	var items []any
	switch typed := value.(type) {
	case []any:
		items = typed
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		items = make([]any, 0, len(keys))
		for _, key := range keys {
			items = append(items, typed[key])
		}
	default:
		return []string{}
	}

	fields := make([]string, 0)
	for _, item := range items {
		if field, ok := item.(string); ok && field != "" {
			fields = append(fields, field)
		}
	}
	return fields
}

func retryAfter(response *Response, payload map[string]any) *int {
	if payload != nil {
		if value, ok := payload["retry_after"]; ok {
			if parsed, ok := coerceInt(value); ok {
				return &parsed
			}
		}
	}
	if parsed, ok := coerceInt(response.Header("Retry-After")); ok {
		return &parsed
	}
	return nil
}

func coerceInt(value any) (int, bool) {
	switch typed := value.(type) {
	case bool, nil:
		return 0, false
	case int:
		return typed, true
	case int8:
		return int(typed), true
	case int16:
		return int(typed), true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case uint:
		return int(typed), true
	case uint8:
		return int(typed), true
	case uint16:
		return int(typed), true
	case uint32:
		return int(typed), true
	case uint64:
		return int(typed), true
	case float32:
		if math.IsNaN(float64(typed)) || math.IsInf(float64(typed), 0) {
			return 0, false
		}
		return int(typed), true
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return 0, false
		}
		return int(typed), true
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return 0, false
		}
		parsed, err := strconv.ParseFloat(text, 64)
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return 0, false
		}
		return int(parsed), true
	default:
		return 0, false
	}
}

func asText(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case bool:
		if typed {
			return "1"
		}
		return ""
	case nil:
		return ""
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	default:
		return fmt.Sprint(value)
	}
}
