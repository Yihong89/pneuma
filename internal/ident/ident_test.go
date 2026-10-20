package ident

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/tailcfg"
)

type fakeWhoIs struct{ resp *apitype.WhoIsResponse }

func (f fakeWhoIs) WhoIs(context.Context, string) (*apitype.WhoIsResponse, error) {
	return f.resp, nil
}

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
			tc.ts.Client = fakeWhoIs{tc.resp}
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
	ts := Tailscale{Client: fakeWhoIs{peer("mac-mini", "macOS", "me@x.com")}}
	s, err := ts.Identify(httptest.NewRequest("GET", "/", nil))
	if err != nil || s.OS != "macos" {
		t.Fatalf("got %+v, %v", s, err)
	}
}
