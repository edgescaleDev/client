# client

The Go client for Edgescale's products. It's for your backend:

- **billing and OTP** take your project's platform key
- **auth's server API** takes a server app's secret, or the key with auth access

```bash
go get github.com/edgescaleDev/client
```

It needs Go 1.26 or newer. Versions are `v0.x` until the API is called stable, so a minor version may change it.

## A first call

```go
c, err := client.New("k3x9m2q7pa", client.WithKey(os.Getenv("EDGESCALE_KEY")))
if err != nil {
	return err
}
plans, err := client.Decode[[]billing.Plan](c.Billing.ListPlans(ctx, nil))
```

`k3x9m2q7pa` is the project's uid, from its Settings in the dashboard. Every call goes to the project's own host:
`{uid}.projects.apisdomain.dev` for billing and OTP, and `{uid}.auth.apisdomain.dev` for auth.

Each product is a package generated from its public spec, in [`spec/`](spec): `billing`, `otp` and `auth`. Their calls
return an `*http.Response`. `client.Decode` reads the answer into the type you name, or returns an `*client.Error`.

## Keys and staging

- **A project is one environment.** Build and test against a second project, such as staging, with its own uid and
  keys.
- **A test key, `eds_ak_test_…`,** is for a product's test mode, such as OTP's test numbers. Billing has no test
  mode, and refuses one with a 403.

## Auth's server API

```go
c, err := client.New(uid, client.WithServerSecret(os.Getenv("EDGESCALE_SERVER_SECRET")))
user, err := client.Decode[auth.User](c.Auth.AdminGetUser(ctx, id))
```

A server app's secret, `eds_as_…`, is made in the project's Auth tab. The client sends it as the bearer on
`/v1/admin`, and as `Edgescale-Server-Secret` on the routes your backend calls for its users, such as sign-in.

- **Say who you're calling for** with the call's params. Auth records only what you send, never your server's own
  address:

  ```go
  addr := realIP(r) // the person's address, as your server saw it
  tokens, err := client.Decode[auth.Tokens](c.Auth.SignIn(ctx, &auth.SignInParams{EdgescaleClientAddress: &addr}, body))
  ```

- **Or use the platform key,** with auth access set in the dashboard. With no secret, the client sends the key in the
  secret's place:
  - `read` looks a user up, lists their sessions, and searches
  - `write` does the rest, signing a person in included
  - a test key, `eds_ak_test_…`, is refused with a 403: auth has no test mode
- **One client can hold both credentials:** `client.New(uid, client.WithKey(…), client.WithServerSecret(…))`. Auth then
  gets the secret.

## Errors

A failed call is an `*client.Error`, with the status and the answer's errors. Branch on an error's code, never its
message:

```go
var e *client.Error
if errors.As(err, &e) && e.Has(1007) {
	time.Sleep(e.RetryAfter) // too many: 429, with Retry-After
}
```

| Code | Status | What |
|---|---|---|
| 1000 | 500 | our own failure |
| 1001 | 401 | a missing, wrong or revoked key or secret |
| 1002 | 403 | not allowed, such as a key without the access, or a test key where there's no test mode |
| 1003 | 404 | not found |
| 1004 | 400 | a request that can't be read |
| 1005 | 422 | an invalid field |
| 1007 | 429 | too many, with `RetryAfter` |
| 1008 | 503 | briefly unavailable, with `RetryAfter` |

A failure from in front of the API, such as a proxy's 502, is an `*client.Error` with no errors.

The client doesn't retry. Pass your own `http.Client` with `client.WithHTTPClient`, with its timeouts and retries.

## Notices and paging

`client.Read` returns the whole answer:
- `Result`
- `Messages`: notices that aren't failures, such as auth's 3000 when it didn't store a header you sent
- `Page`: a list's paging

```go
page := 1
a, err := client.Read[[]billing.Plan](c.Billing.ListPlans(ctx, &billing.ListPlansParams{Page: &page}))
// a.Page.TotalPages is the last page
```

## Webhooks

Billing's webhooks to you are [Standard Webhooks](https://www.standardwebhooks.com). Verify them with its Go library,
`github.com/standard-webhooks/standard-webhooks/libraries/go`:

```go
wh, err := standardwebhooks.NewWebhook(os.Getenv("EDGESCALE_WEBHOOK_SECRET"))
// in the handler:
body, _ := io.ReadAll(r.Body)
if err := wh.Verify(body, r.Header); err != nil {
	http.Error(w, "bad signature", http.StatusBadRequest)
	return
}
```

## The specs

[`spec/`](spec) holds each product's public spec, in OpenAPI 3.0. The packages are generated from them with
`go generate`, so you can generate a client in another language from the same files.

## License

Apache 2.0. See [LICENSE](LICENSE).
