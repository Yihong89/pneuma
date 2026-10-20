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
	}{
		{name: "personal device", resp: peer("mac-mini", "macOS", "me@x.com"), device: "mac-mini"},
		{name: "tagged node rejected", resp: peer("prod", "linux", "tagged-devices", "tag:server"), wantErr: true},
		{name: "tagged node allowed when enabled", ts: Tailscale{AllowTagged: true}, resp: peer("prod", "linux", "tagged-devices", "tag:server"), device: "prod"},
		{name: "allowlist match", ts: Tailscale{AllowedUsers: []string{"me@x.com"}}, resp: peer("phone", "android", "me@x.com"), device: "phone"},
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
			if s.Device != tc.device {
				t.Fatalf("device = %q, want %q", s.Device, tc.device)
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
	st := &ipnstate.Status{
		Peer: map[key.NodePublic]*ipnstate.PeerStatus{
			key.NewNode().Public(): {DNSName: "mac-mini.x.ts.net.", OS: "macOS", Online: true, UserID: 1},
			key.NewNode().Public(): {DNSName: "phone.x.ts.net.", OS: "android", UserID: 1},
			key.NewNode().Public(): {DNSName: "prod.x.ts.net.", OS: "linux", Online: true, UserID: 2, Tags: &tags},
			key.NewNode().Public(): {DNSName: "friend.x.ts.net.", OS: "linux", Online: true, UserID: 3},
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
	if len(devs) != 2 || devs[0].Name != "mac-mini" || !devs[0].Online || devs[1].Name != "phone" || devs[1].OS != "android" {
		t.Fatalf("devices = %+v", devs)
	}
}
