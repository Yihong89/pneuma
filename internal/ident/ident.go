// Package ident works out which device is making a request.
package ident

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"tailscale.com/client/tailscale/apitype"

	"github.com/ericpastorm/pneuma/internal/store"
)

// ErrForbidden means the peer is known but not allowed in.
var ErrForbidden = errors.New("forbidden")

// Identifier resolves the sender behind an HTTP request.
type Identifier interface {
	Identify(r *http.Request) (store.Sender, error)
}

// WhoIser is the part of tailscale.com/client/local.Client we need.
type WhoIser interface {
	WhoIs(ctx context.Context, remoteAddr string) (*apitype.WhoIsResponse, error)
}

// Tailscale identifies peers through the tailnet's WhoIs API, so there is
// nothing to log in to: the device is the credential.
type Tailscale struct {
	Client WhoIser
	// AllowTagged lets tagged nodes (servers, CI boxes...) in. Off by default
	// because tagged nodes are not owned by a person.
	AllowTagged bool
	// AllowedUsers, when non-empty, restricts access to these login names.
	AllowedUsers []string
}

func (t *Tailscale) Identify(r *http.Request) (store.Sender, error) {
	who, err := t.Client.WhoIs(r.Context(), r.RemoteAddr)
	if err != nil {
		return store.Sender{}, err
	}
	if who.Node == nil || who.UserProfile == nil {
		return store.Sender{}, ErrForbidden
	}
	if who.Node.IsTagged() && !t.AllowTagged {
		return store.Sender{}, ErrForbidden
	}
	login := who.UserProfile.LoginName
	if len(t.AllowedUsers) > 0 && !slices.Contains(t.AllowedUsers, login) {
		return store.Sender{}, ErrForbidden
	}
	device := who.Node.ComputedName
	if device == "" {
		device, _, _ = strings.Cut(who.Node.Name, ".")
	}
	var os string
	if who.Node.Hostinfo.Valid() {
		os = who.Node.Hostinfo.OS()
	}
	return store.Sender{Device: device, OS: strings.ToLower(os), User: login}, nil
}

// Dev is used outside a tailnet for local development. The device name can
// be overridden per request with ?as=<name> so two browser tabs can pretend
// to be two machines.
type Dev struct{}

func (Dev) Identify(r *http.Request) (store.Sender, error) {
	s := store.Sender{Device: "dev-laptop", OS: "linux", User: "dev@localhost"}
	if as := r.URL.Query().Get("as"); as != "" {
		s.Device = as
	}
	switch {
	case strings.Contains(s.Device, "mac"):
		s.OS = "macos"
	case strings.Contains(s.Device, "phone"), strings.Contains(s.Device, "android"):
		s.OS = "android"
	}
	return s, nil
}

type ctxKey struct{}

// WithSender stores the sender in a context.
func WithSender(ctx context.Context, s store.Sender) context.Context {
	return context.WithValue(ctx, ctxKey{}, s)
}

// SenderFrom returns the sender stored by WithSender.
func SenderFrom(ctx context.Context) store.Sender {
	s, _ := ctx.Value(ctxKey{}).(store.Sender)
	return s
}
