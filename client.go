// Package client calls Edgescale's products from a backend: billing and OTP
// with the project's platform key, and auth's server API with a server app's
// secret.
//
//	c, err := client.New("k3x9m2q7pa", client.WithKey(os.Getenv("EDGESCALE_KEY")))
//	plans, err := client.Decode[[]billing.Plan](c.Billing.ListPlans(ctx, nil))
//
// Each product is a package generated from its public spec, in spec/.
package client

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/edgescaleDev/client/auth"
	"github.com/edgescaleDev/client/billing"
	"github.com/edgescaleDev/client/otp"
)

// Client is one project's products, each the generated client of its spec.
type Client struct {
	Billing billing.ClientInterface // takes the key
	OTP     otp.ClientInterface     // takes the key
	Auth    auth.ClientInterface    // takes the server secret, or the key once auth checks keys
}

// Option is one of New's settings.
type Option func(*options)

type options struct {
	key, secret, zone string
	hc                *http.Client
}

// WithKey is the project's platform key, eds_ak_…, or eds_ak_test_… for a
// product's test mode.
func WithKey(key string) Option { return func(o *options) { o.key = key } }

// WithServerSecret is a server app's secret, eds_as_…, from the project's Auth
// tab.
func WithServerSecret(secret string) Option { return func(o *options) { o.secret = secret } }

// WithZone is the zone under the project's hosts: apisdomain.dev unless set,
// and dev.edgescale.dev for dev.
func WithZone(zone string) Option { return func(o *options) { o.zone = zone } }

// WithHTTPClient sends every call through hc, http.DefaultClient unless set;
// nil keeps the default. Its timeouts and retries are the caller's.
func WithHTTPClient(hc *http.Client) Option {
	return func(o *options) {
		if hc != nil {
			o.hc = hc
		}
	}
}

// New is a client for one project, by its uid. It needs a key, a server
// secret, or both.
func New(uid string, opts ...Option) (*Client, error) {
	o := &options{zone: "apisdomain.dev", hc: http.DefaultClient}
	for _, opt := range opts {
		opt(o)
	}
	if o.key == "" && o.secret == "" {
		return nil, errors.New("client: New needs WithKey, WithServerSecret, or both")
	}
	if uid == "" || strings.Trim(uid, "abcdefghijklmnopqrstuvwxyz0123456789") != "" {
		return nil, errors.New("client: " + uid + " isn't a project's uid")
	}
	projects := "https://" + uid + ".projects." + o.zone
	b, err := billing.NewClient(projects, billing.WithHTTPClient(o.hc), billing.WithRequestEditorFn(o.bearer))
	if err != nil {
		return nil, err
	}
	t, err := otp.NewClient(projects, otp.WithHTTPClient(o.hc), otp.WithRequestEditorFn(o.bearer))
	if err != nil {
		return nil, err
	}
	a, err := auth.NewClient("https://"+uid+".auth."+o.zone, auth.WithHTTPClient(o.hc), auth.WithRequestEditorFn(o.auth))
	if err != nil {
		return nil, err
	}
	return &Client{Billing: b, OTP: t, Auth: a}, nil
}

// bearer sends the key, which billing and OTP take.
func (o *options) bearer(_ context.Context, r *http.Request) error {
	if o.key != "" {
		r.Header.Set("Authorization", "Bearer "+o.key)
	}
	return nil
}

// auth sends the server secret: as the bearer on the server API, /v1/admin,
// and as Edgescale-Server-Secret on the routes a backend calls for its users,
// so auth never records the backend's own address as a person's. A key alone
// goes as the server API's bearer, which auth refuses until it checks keys.
func (o *options) auth(_ context.Context, r *http.Request) error {
	admin := strings.HasPrefix(r.URL.Path, "/v1/admin/")
	switch {
	case o.secret != "" && admin:
		r.Header.Set("Authorization", "Bearer "+o.secret)
	case o.secret != "":
		r.Header.Set("Edgescale-Server-Secret", o.secret)
	case admin:
		r.Header.Set("Authorization", "Bearer "+o.key)
	}
	return nil
}
