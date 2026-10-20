# Setalabs Go SDK

Client for the Setalabs gateway. Each API has its own uuid, key, and set of endpoints. The gateway address is shared.

The key secret is used only to sign requests. It is never sent.

## Installation

```bash
go get github.com/setalabs/setalabs-go-sdk
```

Until the package is published, depend on it from a local path:

```go
require github.com/setalabs/setalabs-go-sdk v0.0.0

replace github.com/setalabs/setalabs-go-sdk => ../setalabs-go-sdk
```

## Requests

The default gateway address is `setalabs.GatewayURL` (`https://gw.setalabs.com`). Pass `WithGateway` to use another one. An OpenAPI server URL already includes the API uuid; this SDK takes the uuid separately.

```go
client, err := setalabs.New(
    "05141812-7772-403e-9ca6-5e8e8bf22730",
    "your-key",
    "your-secret",
)
if err != nil {
    return err
}
defer client.Close()

local, err := setalabs.New(
    "05141812-7772-403e-9ca6-5e8e8bf22730",
    "your-key",
    "your-secret",
    setalabs.WithGateway("http://localhost:8080"),
)

response, err := client.Get(ctx, "/catalog", setalabs.Params{
    {Name: "language", Value: "en"},
}, nil)
if err != nil {
    return err
}
data, err := response.JSON()
```

The path is the endpoint path from the OpenAPI document (`/catalog`, `/product/{id}`, `/search`), without the `/v1/api/{uuid}` prefix. Substitute path parameters yourself: `/product/12345`.

Another API on the same gateway:

```go
other, err := client.WithAPI("another-api-uuid", "other-key", "other-secret")
```

If the key has no secret, pass an empty string. The request is not signed.

```go
client.Post(ctx, "/orders", map[string]any{"sku": "A1"}, nil, nil)

client.Request(ctx, "PUT", "/orders/1", setalabs.Request{
    JSON:    setalabs.JSON(map[string]any{"status": "paid"}),
    Query:   setalabs.Params{{Name: "language", Value: "en"}},
    Timeout: 10 * time.Second,
})
```

`Params` keeps parameter order. A map is also accepted; its keys are encoded in sorted order because Go maps have no order. A string is sent as the raw query, without the leading `?`. JSON object keys are encoded in sorted order. The signature covers the path and query, not the body.

A successful response is the upstream body and status, unchanged. The gateway returns the request id in `X-Request-Id`:

```go
response.StatusCode
response.RequestID()
response.JSON()
```

`Close` releases the shared connection pool created by `New`. A transport passed with `WithTransport` is left open.

## Signature

When a secret is set, every request includes:

- `X-Api-Key`
- `X-Api-Timestamp` — Unix time in seconds
- `X-Api-Sign` — `HMAC-SHA256(key + secret + timestamp + path?query, secret)`, lowercase hex

`path?query` is the path the gateway sees, plus the raw query string. For example: `/v1/api/{uuid}/catalog?language=en`. Parameter order is preserved for `Params`. The query string uses the same encoding as PHP `http_build_query` with `PHP_QUERY_RFC3986`.

## Errors

A response with status 400 or higher returns an error. The message comes from the gateway `reason`. Every API error exposes `StatusCode`, `ErrorCode`, `Reason`, `RequestID`, `MissingFields`, `RetryAfter`, and the original `Response`. Use `errors.As` to read them.

| HTTP | `error` | Error | When |
| --- | --- | --- | --- |
| 400 | `bad_request` | `BadRequestError` | The API has no base URL |
| 401 | `unauthorized` | `UnauthorizedError` | Missing or invalid key, missing or invalid signature, missing timestamp |
| 403 | `forbidden` | `ForbiddenError` | Key is blocked, subscription has expired, no active tariff, or the key belongs to another API |
| 404 | `not_found` | `NotFoundError` | API or endpoint was not found |
| 422 | `validation_error` | `ValidationError` | Required fields are missing; names are in `MissingFields` |
| 429 | `rate_limited` | `RateLimitError` | Plan RPS limit exceeded; wait `RetryAfter` seconds |
| 429 | `usage_limit_exceeded` | `UsageLimitError` | Call quota for the period is exhausted |
| 500 | `internal_error` | `ServerError` | Gateway internal error |
| 502 | `bad_gateway` | `BadGatewayError` | Upstream is unavailable |

A network failure returns `TransportError`. `Error` is the base type for gateway and transport errors. Invalid client arguments return `ArgumentError`.

`RateLimitError` and `UsageLimitError` can also be read as `TooManyRequestsError`.

```go
response, err := client.Get(ctx, "/search", setalabs.Params{
    {Name: "text", Value: "phone"},
    {Name: "page", Value: 1},
    {Name: "limit", Value: 24},
}, nil)
if err != nil {
    var validation *setalabs.ValidationError
    var rate *setalabs.RateLimitError
    var apiErr *setalabs.APIError
    switch {
    case errors.As(err, &validation):
        _ = validation.MissingFields
    case errors.As(err, &rate):
        wait := 1
        if rate.RetryAfter != nil {
            wait = *rate.RetryAfter
        }
        time.Sleep(time.Duration(wait) * time.Second)
    case errors.As(err, &apiErr):
        _ = apiErr.StatusCode
        _ = apiErr.Reason
        _ = apiErr.RequestID
    default:
        return err
    }
}
data, err := response.JSON()
```
