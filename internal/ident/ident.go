// Package ident works out which device is making a request.
package ident

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/ipn/ipnstate"

	"github.com/ericpastorm/pneuma/internal/store"
)

// ErrForbidden means the peer is known but not allowed in.
var ErrForbidden = errors.New("forbidden")

// Identifier resolves the sender behind an HTTP request.
type Identifier interface {
	Identify(r *http.Request) (store.Sender, error)
}

// LocalClient is the part of tailscale.com/client/local.Client we need.
type LocalClient interface {
	WhoIs(ctx context.Context, remoteAddr string) (*apitype.WhoIsResponse, error)
	Status(ctx context.Context) (*ipnstate.Status, error)
}

// Device is a machine on the tailnet that may use pneuma.
type Device struct {
	ID     string `json:"id"` // stable node ID
	Name   string `json:"name"`
	OS     string `json:"os"`
	Online bool   `json:"online"` // connected to the tailnet
}

// DeviceLister lists the devices that could talk to pneuma.
type DeviceLister interface {
	Devices(ctx context.Context) ([]Device, error)
}

// Tailscale identifies peers through the tailnet's WhoIs API, so there is
// nothing to log in to: the device is the credential.
type Tailscale struct {
	Client LocalClient
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
	if !t.userAllowed(login) {
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
	return store.Sender{ID: string(who.Node.StableID), Device: device, OS: strings.ToLower(os), User: login}, nil
}

func (t *Tailscale) userAllowed(login string) bool {
	return len(t.AllowedUsers) == 0 || slices.Contains(t.AllowedUsers, login)
}

// Devices lists the tailnet devices that pass the same rules as Identify,
// so pneuma never shows a machine that couldn't actually use it.
func (t *Tailscale) Devices(ctx context.Context) ([]Device, error) {
	st, err := t.Client.Status(ctx)
	if err != nil {
		return nil, err
	}
	var out []Device
	for _, p := range st.Peer {
		if p.Tags != nil && p.Tags.Len() > 0 && !t.AllowTagged {
			continue
		}
		// Fail closed: with an allowlist, a peer whose owner we can't
		// resolve is not shown.
		u, ok := st.User[p.UserID]
		if len(t.AllowedUsers) > 0 && (!ok || !t.userAllowed(u.LoginName)) {
			continue
		}
		name, _, _ := strings.Cut(p.DNSName, ".")
		if name == "" {
			name = p.HostName
		}
		out = append(out, Device{ID: string(p.ID), Name: name, OS: strings.ToLower(p.OS), Online: p.Online})
	}
	slices.SortFunc(out, func(a, b Device) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
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
	s.ID = DevID(s.Device)
	switch {
	case strings.Contains(s.Device, "mac"):
		s.OS = "macos"
	case strings.Contains(s.Device, "phone"), strings.Contains(s.Device, "android"):
		s.OS = "android"
	}
	return s, nil
}

// DevID is the stable ID a device gets in dev mode.
func DevID(device string) string { return "dev:" + device }

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
