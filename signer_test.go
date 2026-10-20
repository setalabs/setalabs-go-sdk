package setalabs

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestSignMatchesGatewayAlgorithm(t *testing.T) {
	apiKey := "key-1"
	secret := "secret-1"
	timestamp := "1700000000"
	pathWithQuery := "/v1/api/uuid/items?q=1"

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(apiKey + secret + timestamp + pathWithQuery))
	expected := hex.EncodeToString(mac.Sum(nil))

	if got := Sign(apiKey, secret, timestamp, pathWithQuery); got != expected {
		t.Fatalf("Sign() = %s, want %s", got, expected)
	}
	if expected != lower(expected) || len(expected) != 64 {
		t.Fatalf("signature %s is not 64 lowercase hex characters", expected)
	}
}

func lower(value string) string {
	out := make([]byte, len(value))
	for i := 0; i < len(value); i++ {
		char := value[i]
		if char >= 'A' && char <= 'Z' {
			char += 'a' - 'A'
		}
		out[i] = char
	}
	return string(out)
}
