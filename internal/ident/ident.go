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

// Device is an entry in the who's-who of pneuma: one per person, keyed by
// account. (The type name and JSON shape stay for the web app.)
type Device struct {
	ID     string `json:"id"` // account ID ("user:<login>")
	Name   string `json:"name"`
	OS     string `json:"os"`     // every OS that person's devices run
	Online bool   `json:"online"` // any of their devices is connected
}

// DeviceLister lists the devices that could talk to pneuma.
type DeviceLister interface {
	Devices(ctx context.Context) ([]Device, error)
}

// Tailscale identifies peers through the tailnet's WhoIs API, so there is
// nothing to log in to: the device's owner account is the credential.
type Tailscale struct {
	Client LocalClient
	// AllowTagged lets tagged nodes (servers, CI boxes...) in. Off by default
	// because tagged nodes are not owned by a person.
	AllowTagged bool
	// AllowedUsers, when non-empty, restricts access to these login names.
	AllowedUsers []string
}

// accountID is the stable membership key: a person, identified by their
// lowercased tailnet login. Multiple devices of one account share it, so
// chats and presence are per person rather than per machine.
func accountID(login string) string { return "user:" + strings.ToLower(login) }

// personName is the display label for an account: the local part of its
// email, which aliases can then prettify ("zhangyhzju" → "爸爸").
func personName(login string) string {
	name, _, _ := strings.Cut(login, "@")
	if name == "" {
		return login
	}
	return name
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
	return store.Sender{ID: accountID(login), Device: device, OS: strings.ToLower(os), User: login}, nil
}

func (t *Tailscale) userAllowed(login string) bool {
	if len(t.AllowedUsers) == 0 {
		return true
	}
	login = strings.ToLower(login)
	return slices.ContainsFunc(t.AllowedUsers, func(u string) bool {
		return strings.ToLower(u) == login
	})
}

// Devices lists the people whose devices may use pneuma, aggregating every
// tailnet peer under its owner's account, with the same rules as Identify.
func (t *Tailscale) Devices(ctx context.Context) ([]Device, error) {
	st, err := t.Client.Status(ctx)
	if err != nil {
		return nil, err
	}
	type person struct {
		dev  Device
		oses map[string]bool
	}
	by := map[string]*person{}
	for _, p := range st.Peer {
		if p.Tags != nil && p.Tags.Len() > 0 && !t.AllowTagged {
			continue
		}
		// Fail closed: a peer whose owner we can't resolve is nobody's
		// device and gets no seat at the table.
		u, ok := st.User[p.UserID]
		if !ok || u.LoginName == "" || !t.userAllowed(u.LoginName) {
			continue
		}
		login := u.LoginName
		id := accountID(login)
		pr, seen := by[id]
		if !seen {
			pr = &person{dev: Device{ID: id, Name: personName(strings.ToLower(login))}, oses: map[string]bool{}}
			by[id] = pr
		}
		pr.dev.Online = pr.dev.Online || p.Online
		if p.OS != "" {
			pr.oses[strings.ToLower(p.OS)] = true
		}
	}
	out := make([]Device, 0, len(by))
	for _, pr := range by {
		oses := make([]string, 0, len(pr.oses))
		for o := range pr.oses {
			oses = append(oses, o)
		}
		slices.Sort(oses)
		pr.dev.OS = strings.Join(oses, "/")
		out = append(out, pr.dev)
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

// DevName is the fake device name a dev-mode ID carries, or "" if the ID
// is not a dev one.
func DevName(id string) string {
	if name, ok := strings.CutPrefix(id, "dev:"); ok {
		return name
	}
	return ""
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
