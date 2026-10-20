package setalabs

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// Sign returns the X-Api-Sign value expected by the gateway.
//
// The signature is HMAC-SHA256 of apiKey + secret + timestamp + pathWithQuery,
// keyed by the secret, as lowercase hex. pathWithQuery is the decoded request
// path plus the raw query string, for example /v1/api/{uuid}/items?page=1.
func Sign(apiKey, secret, timestamp, pathWithQuery string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(apiKey + secret + timestamp + pathWithQuery))
	return hex.EncodeToString(mac.Sum(nil))
}
