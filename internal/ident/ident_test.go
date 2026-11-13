package ident

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
	"tailscale.com/types/views"
)

type fakeWhoIs struct {
	resp   *apitype.WhoIsResponse
	status *ipnstate.Status
}

func (f fakeWhoIs) WhoIs(context.Context, string) (*apitype.WhoIsResponse, error) {
	return f.resp, nil
}

func (f fakeWhoIs) Status(context.Context) (*ipnstate.Status, error) { return f.status, nil }

func peer(name, os, login string, tags ...string) *apitype.WhoIsResponse {
	return &apitype.WhoIsResponse{
		Node: &tailcfg.Node{
			StableID:     tailcfg.StableNodeID("n-" + name),
			Name:         name + ".example.ts.net.",
			ComputedName: name,
			Hostinfo:     (&tailcfg.Hostinfo{OS: os}).View(),
			Tags:         tags,
		},
		UserProfile: &tailcfg.UserProfile{LoginName: login},
	}
}

func TestTailscaleIdentify(t *testing.T) {
	tests := []struct {
		name    string
		ts      Tailscale
		resp    *apitype.WhoIsResponse
		wantErr bool
		device  string
		id      string
	}{
		{name: "personal device", resp: peer("mac-mini", "macOS", "me@x.com"), device: "mac-mini", id: "user:me@x.com"},
		{name: "two machines of one account share an ID", resp: peer("phone", "Android", "me@x.com"), device: "phone", id: "user:me@x.com"},
		{name: "tagged node rejected", resp: peer("prod", "linux", "tagged-devices", "tag:server"), wantErr: true},
		{name: "tagged node allowed when enabled", ts: Tailscale{AllowTagged: true}, resp: peer("prod", "linux", "tagged-devices", "tag:server"), device: "prod", id: "user:tagged-devices"},
		{name: "allowlist match", ts: Tailscale{AllowedUsers: []string{"me@x.com"}}, resp: peer("phone", "android", "me@x.com"), device: "phone", id: "user:me@x.com"},
		{name: "allowlist is case insensitive", ts: Tailscale{AllowedUsers: []string{"Me@X.com"}}, resp: peer("phone", "android", "ME@x.com"), device: "phone", id: "user:me@x.com"},
		{name: "allowlist miss", ts: Tailscale{AllowedUsers: []string{"me@x.com"}}, resp: peer("guest", "linux", "friend@y.com"), wantErr: true},
		{name: "missing node", resp: &apitype.WhoIsResponse{}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.ts.Client = fakeWhoIs{resp: tc.resp}
			s, err := tc.ts.Identify(httptest.NewRequest("GET", "/", nil))
			if tc.wantErr {
				if !errors.Is(err, ErrForbidden) {
					t.Fatalf("err = %v, want ErrForbidden", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if s.Device != tc.device || s.ID != tc.id {
				t.Fatalf("got %q (%q), want %q (%q)", s.Device, s.ID, tc.device, tc.id)
			}
		})
	}
}

func TestOSIsLowercased(t *testing.T) {
	ts := Tailscale{Client: fakeWhoIs{resp: peer("mac-mini", "macOS", "me@x.com")}}
	s, err := ts.Identify(httptest.NewRequest("GET", "/", nil))
	if err != nil || s.OS != "macos" {
		t.Fatalf("got %+v, %v", s, err)
	}
}

func TestDevices(t *testing.T) {
	tags := views.SliceOf([]string{"tag:server"})
	p1, p2, p3, p4, p5 := key.NewNode(), key.NewNode(), key.NewNode(), key.NewNode(), key.NewNode()
	st := &ipnstate.Status{
		Peer: map[key.NodePublic]*ipnstate.PeerStatus{
			p1.Public(): {ID: "n-mac", DNSName: "mac-mini.x.ts.net.", OS: "macOS", Online: true, UserID: 1},
			p2.Public(): {DNSName: "phone.x.ts.net.", OS: "android", UserID: 1},
			p3.Public(): {DNSName: "prod.x.ts.net.", OS: "linux", Online: true, UserID: 2, Tags: &tags},
			p4.Public(): {DNSName: "friend.x.ts.net.", OS: "linux", Online: true, UserID: 3},
			p5.Public(): {DNSName: "unknown-owner.x.ts.net.", OS: "linux", Online: true, UserID: 4},
		},
		User: map[tailcfg.UserID]tailcfg.UserProfile{
			1: {LoginName: "me@x.com"},
			2: {LoginName: "tagged-devices"},
			3: {LoginName: "friend@y.com"},
		},
	}
	ts := Tailscale{Client: fakeWhoIs{status: st}, AllowedUsers: []string{"me@x.com"}}
	devs, err := ts.Devices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 1 {
		t.Fatalf("devices = %+v", devs)
	}
	if devs[0].ID != "user:me@x.com" || devs[0].Name != "me" || !devs[0].Online || devs[0].OS != "android/macos" {
		t.Fatalf("aggregated account = %+v", devs[0])
	}

	// Without an allowlist every resolvable account gets one row; the
	// unowned peer and the tagged server stay out.
	open := Tailscale{Client: fakeWhoIs{status: st}}
	devs, err = open.Devices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 2 || devs[0].ID != "user:friend@y.com" || devs[1].ID != "user:me@x.com" {
		t.Fatalf("open devices = %+v", devs)
	}
}
