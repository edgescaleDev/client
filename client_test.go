package client_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/edgescaleDev/client"
	"github.com/edgescaleDev/client/auth"
	"github.com/edgescaleDev/client/billing"
	"github.com/edgescaleDev/client/otp"
)

// recorder keeps each request it's sent, and answers every one with status
// and body.
type recorder struct {
	reqs   []*http.Request
	status int
	header http.Header
	body   string
}

func (r *recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	r.reqs = append(r.reqs, req)
	h := r.header
	if h == nil {
		h = http.Header{}
	}
	return &http.Response{StatusCode: r.status, Header: h, Body: io.NopCloser(strings.NewReader(r.body)), Request: req}, nil
}

const ok = `{"success":true,"errors":[],"messages":[],"result":null}`

// tracked is a body that says whether it was closed.
type tracked struct {
	io.Reader
	closed bool
}

func (b *tracked) Close() error { b.closed = true; return nil }

// TestHeaders checks where each product's calls go, and that each gets the
// credential it takes: the key on billing and OTP; the server secret as the
// bearer on auth's server API and as Edgescale-Server-Secret on the rest.
func TestHeaders(t *testing.T) {
	ctx := context.Background()
	newClient := func(t *testing.T, opts ...client.Option) (*client.Client, *recorder) {
		t.Helper()
		r := &recorder{status: 200, body: ok}
		c, err := client.New("k3x9m2q7pa", append(opts, client.WithHTTPClient(&http.Client{Transport: r}))...)
		if err != nil {
			t.Fatal(err)
		}
		return c, r
	}
	last := func(r *recorder) *http.Request { return r.reqs[len(r.reqs)-1] }

	t.Run("a key, on billing and OTP", func(t *testing.T) {
		c, r := newClient(t, client.WithKey("eds_ak_one"))
		if _, err := client.Decode[[]billing.Plan](c.Billing.ListPlans(ctx, nil)); err != nil {
			t.Fatal(err)
		}
		q := last(r)
		if q.URL.String() != "https://k3x9m2q7pa.projects.apisdomain.dev/v1/billing/plans" || q.Header.Get("Authorization") != "Bearer eds_ak_one" {
			t.Fatalf("billing got %s with %q", q.URL, q.Header.Get("Authorization"))
		}
		if _, err := client.Decode[otp.SentCode](c.OTP.SendCode(ctx, otp.NewCode{Phone: "+9647701234567"})); err != nil {
			t.Fatal(err)
		}
		q = last(r)
		if q.URL.String() != "https://k3x9m2q7pa.projects.apisdomain.dev/v1/otp/codes" || q.Header.Get("Authorization") != "Bearer eds_ak_one" {
			t.Fatalf("otp got %s with %q", q.URL, q.Header.Get("Authorization"))
		}
	})

	t.Run("a secret, on auth", func(t *testing.T) {
		c, r := newClient(t, client.WithServerSecret("eds_as_two"))
		if _, err := client.Decode[auth.User](c.Auth.AdminGetUser(ctx, "123")); err != nil {
			t.Fatal(err)
		}
		q := last(r)
		if q.URL.String() != "https://k3x9m2q7pa.auth.apisdomain.dev/v1/admin/users/123" ||
			q.Header.Get("Authorization") != "Bearer eds_as_two" || q.Header.Get("Edgescale-Server-Secret") != "" {
			t.Fatalf("the server API got %s with %v", q.URL, q.Header)
		}

		addr, agent := "203.0.113.7", "Mozilla/5.0 (KHTML, like Gecko)"
		params := &auth.SignInParams{EdgescaleClientAddress: &addr, EdgescaleClientAgent: &agent}
		if _, err := client.Decode[auth.Tokens](c.Auth.SignIn(ctx, params, auth.SignIn{ClientId: "app"})); err != nil {
			t.Fatal(err)
		}
		q = last(r)
		if q.URL.Path != "/v1/sessions" || q.Header.Get("Edgescale-Server-Secret") != "eds_as_two" || q.Header.Get("Authorization") != "" ||
			q.Header.Get("Edgescale-Client-Address") != addr || q.Header.Get("Edgescale-Client-Agent") != agent ||
			q.Header.Get("Edgescale-Client-City") != "" {
			t.Fatalf("a sign-in for a person got %s with %v", q.URL, q.Header)
		}
	})

	t.Run("both: billing gets the key, and auth the secret", func(t *testing.T) {
		c, r := newClient(t, client.WithKey("eds_ak_one"), client.WithServerSecret("eds_as_two"))
		c.Billing.ListPlans(ctx, nil)
		c.Auth.AdminGetUser(ctx, "123")
		c.Auth.SignIn(ctx, &auth.SignInParams{}, auth.SignIn{ClientId: "app"})
		if a, b := r.reqs[0].Header.Get("Authorization"), r.reqs[1].Header.Get("Authorization"); a != "Bearer eds_ak_one" || b != "Bearer eds_as_two" {
			t.Fatalf("billing %q, auth %q", a, b)
		}
		if h := r.reqs[2].Header; h.Get("Edgescale-Server-Secret") != "eds_as_two" || h.Get("Authorization") != "" {
			t.Fatalf("a sign-in got %v", h)
		}
	})

	t.Run("a key alone, on auth: the bearer on the server API, the server secret on the rest", func(t *testing.T) {
		c, r := newClient(t, client.WithKey("eds_ak_one"))
		c.Auth.AdminGetUser(ctx, "123")
		c.Auth.SignIn(ctx, &auth.SignInParams{}, auth.SignIn{ClientId: "app"})
		if h := r.reqs[0].Header; h.Get("Authorization") != "Bearer eds_ak_one" || h.Get("Edgescale-Server-Secret") != "" {
			t.Fatalf("the server API got %v", h)
		}
		if h := r.reqs[1].Header; h.Get("Edgescale-Server-Secret") != "eds_ak_one" || h.Get("Authorization") != "" {
			t.Fatalf("a sign-in got %v", h)
		}
	})

	t.Run("dev's zone", func(t *testing.T) {
		c, r := newClient(t, client.WithKey("eds_ak_one"), client.WithZone("dev.edgescale.dev"))
		c.Billing.ListPlans(ctx, nil)
		c.Auth.AdminGetUser(ctx, "123")
		if a, b := r.reqs[0].URL.Host, r.reqs[1].URL.Host; a != "k3x9m2q7pa.projects.dev.edgescale.dev" || b != "k3x9m2q7pa.auth.dev.edgescale.dev" {
			t.Fatalf("hosts %s and %s", a, b)
		}
	})

	t.Run("a nil HTTP client keeps the default", func(t *testing.T) {
		c, err := client.New("k3x9m2q7pa", client.WithKey("eds_ak_one"), client.WithHTTPClient(nil))
		if err != nil {
			t.Fatal(err)
		}
		done, cancel := context.WithCancel(ctx)
		cancel() // so the default client never dials
		if _, err := client.Decode[[]billing.Plan](c.Billing.ListPlans(done, nil)); !errors.Is(err, context.Canceled) {
			t.Fatalf("%v", err)
		}
	})

	t.Run("New refuses", func(t *testing.T) {
		if _, err := client.New("k3x9m2q7pa"); err == nil {
			t.Error("no credential")
		}
		for _, uid := range []string{"", "K3X9M2Q7PA", "a.b", "evil.example/x"} {
			if _, err := client.New(uid, client.WithKey("eds_ak_one")); err == nil {
				t.Errorf("the uid %q", uid)
			}
		}
	})
}

// TestDecode reads answers recorded from production on 2026-11-24, and the
// envelope's other shapes.
func TestDecode(t *testing.T) {
	answer := func(status int, header http.Header, body string) (*http.Response, error) {
		if header == nil {
			header = http.Header{}
		}
		return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body))}, nil
	}

	for file, want := range map[string]struct{ status, code int }{
		"billing-401.json":  {401, 1001},
		"otp-401.json":      {401, 1001},
		"notfound-404.json": {404, 1003},
		"auth-401.json":     {401, 1001},
		"signin-401.json":   {401, 1001},
	} {
		b, err := os.ReadFile("testdata/" + file)
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.Decode[[]billing.Plan](answer(want.status, nil, string(b)))
		var e *client.Error
		if !errors.As(err, &e) || e.Status != want.status || !e.Has(want.code) || e.Has(1005) || !strings.Contains(e.Error(), e.Errors[0].Message) {
			t.Errorf("%s: %v", file, err)
		}
	}

	t.Run("a success, with its notices and paging", func(t *testing.T) {
		a, err := client.Read[[]billing.Plan](answer(200, nil, `{"success":true,"errors":[],
			"messages":[{"code":3000,"message":"Edgescale-Client-Address isn't an IP address, so it wasn't stored"}],
			"result":[{"code":"pro","name":"Pro"}],
			"result_info":{"page":1,"per_page":20,"count":1,"total_count":21,"total_pages":2}}`))
		if err != nil || len(a.Result) != 1 || a.Result[0].Code != "pro" || len(a.Messages) != 1 || a.Messages[0].Code != 3000 ||
			a.Page == nil || a.Page.TotalPages != 2 || a.Page.TotalCount != 21 {
			t.Fatalf("%+v, %v", a, err)
		}
		plans, err := client.Decode[[]billing.Plan](answer(200, nil, `{"success":true,"errors":[],"messages":[],"result":[{"code":"pro"}]}`))
		if err != nil || len(plans) != 1 {
			t.Fatalf("%+v, %v", plans, err)
		}
	})

	t.Run("a null result, and no body", func(t *testing.T) {
		if v, err := client.Decode[*auth.Tokens](answer(202, nil, ok)); err != nil || v != nil {
			t.Fatalf("%+v, %v", v, err)
		}
		if _, err := client.Decode[auth.Tokens](answer(204, nil, "")); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("too many, with Retry-After", func(t *testing.T) {
		_, err := client.Decode[otp.SentCode](answer(429, http.Header{"Retry-After": {"60"}},
			`{"success":false,"errors":[{"code":1007,"message":"too many codes"}],"messages":[],"result":null}`))
		var e *client.Error
		if !errors.As(err, &e) || e.RetryAfter != time.Minute || !e.Has(1007) {
			t.Fatalf("%v", err)
		}
	})

	t.Run("a failure in front of the API", func(t *testing.T) {
		_, err := client.Decode[otp.SentCode](answer(526, nil, "<html>Invalid SSL certificate</html>"))
		var e *client.Error
		if !errors.As(err, &e) || e.Status != 526 || len(e.Errors) != 0 || e.Error() != "edgescale: 526" {
			t.Fatalf("%q", err)
		}
	})

	t.Run("the body is read to its end and closed", func(t *testing.T) {
		body := &tracked{Reader: strings.NewReader(ok + strings.Repeat(" ", 32<<10))} // past the decoder's buffer
		if _, err := client.Decode[*auth.Tokens](&http.Response{StatusCode: 200, Header: http.Header{}, Body: body}, nil); err != nil {
			t.Fatal(err)
		}
		if rest, _ := io.ReadAll(body.Reader); len(rest) != 0 || !body.closed {
			t.Fatalf("%d bytes left, closed %v", len(rest), body.closed)
		}
	})

	t.Run("a success that isn't an envelope", func(t *testing.T) {
		_, err := client.Decode[otp.SentCode](answer(200, nil, "<html></html>"))
		var e *client.Error
		if err == nil || errors.As(err, &e) {
			t.Fatalf("%v", err)
		}
	})

	t.Run("the call's own error", func(t *testing.T) {
		gone := errors.New("dial tcp: no route")
		if _, err := client.Decode[otp.SentCode](nil, gone); !errors.Is(err, gone) {
			t.Fatalf("%v", err)
		}
	})
}
