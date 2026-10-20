package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/ericpastorm/pneuma/internal/hub"
	"github.com/ericpastorm/pneuma/internal/ident"
	"github.com/ericpastorm/pneuma/internal/store"
)

func newServer(t *testing.T, id ident.Identifier) *httptest.Server {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	s := &Server{
		Store:     st,
		Hub:       hub.New(),
		Ident:     id,
		Web:       fstest.MapFS{"index.html": {Data: []byte("<h1>pneuma</h1>")}},
		MaxUpload: 1 << 20,
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func multipartBody(t *testing.T, fields map[string]string, files map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		w.WriteField(k, v)
	}
	for name, content := range files {
		fw, err := w.CreateFormFile("file", name)
		if err != nil {
			t.Fatal(err)
		}
		io.WriteString(fw, content)
	}
	w.Close()
	return &buf, w.FormDataContentType()
}

func TestTextRoundTrip(t *testing.T) {
	ts := newServer(t, ident.Dev{})
	res, err := http.Post(ts.URL+"/api/messages?as=mac-mini", "application/json", strings.NewReader(`{"text":"hola"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d", res.StatusCode)
	}

	res, err = http.Get(ts.URL + "/api/messages")
	if err != nil {
		t.Fatal(err)
	}
	var msgs []store.Message
	json.NewDecoder(res.Body).Decode(&msgs)
	if len(msgs) != 1 || msgs[0].Body != "hola" || msgs[0].Device != "mac-mini" || msgs[0].OS != "macos" {
		t.Fatalf("messages = %+v", msgs)
	}

	res, _ = http.Post(ts.URL+"/api/messages", "application/json", strings.NewReader(`{"text":"   "}`))
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty message status = %d", res.StatusCode)
	}
}

func TestUploadDownloadDelete(t *testing.T) {
	ts := newServer(t, ident.Dev{})
	body, ctype := multipartBody(t, nil, map[string]string{"a.txt": "alpha", "../../evil.svg": "<svg onload=alert(1)>"})
	res, err := http.Post(ts.URL+"/api/files", ctype, body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status = %d: %s", res.StatusCode, b)
	}
	var msgs []store.Message
	json.NewDecoder(res.Body).Decode(&msgs)
	if len(msgs) != 2 {
		t.Fatalf("got %d messages", len(msgs))
	}
	byName := map[string]store.Message{}
	for _, m := range msgs {
		byName[m.File.Name] = m
	}
	svg, ok := byName["evil.svg"]
	if !ok {
		t.Fatalf("path components not stripped: %+v", byName)
	}

	res, _ = http.Get(ts.URL + "/f/" + svg.File.ID + "/evil.svg")
	if cd := res.Header.Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment") {
		t.Errorf("svg should download, got %q", cd)
	}
	if csp := res.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "sandbox") {
		t.Errorf("missing sandbox CSP: %q", csp)
	}

	txt := byName["a.txt"]
	res, _ = http.Get(ts.URL + "/f/" + txt.File.ID + "/a.txt")
	b, _ := io.ReadAll(res.Body)
	if string(b) != "alpha" || !strings.HasPrefix(res.Header.Get("Content-Disposition"), "inline") {
		t.Fatalf("download = %q, %q", b, res.Header.Get("Content-Disposition"))
	}

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/messages/"+strconv.FormatInt(txt.ID, 10), nil)
	res, _ = http.DefaultClient.Do(req)
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d", res.StatusCode)
	}
	res, _ = http.Get(ts.URL + "/f/" + txt.File.ID + "/a.txt")
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("download after delete = %d", res.StatusCode)
	}
}

func TestUploadTooLarge(t *testing.T) {
	ts := newServer(t, ident.Dev{})
	body, ctype := multipartBody(t, nil, map[string]string{"big.bin": strings.Repeat("x", 2<<20)})
	res, err := http.Post(ts.URL+"/api/files", ctype, body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d", res.StatusCode)
	}
}

func TestShareTarget(t *testing.T) {
	ts := newServer(t, ident.Dev{})
	body, ctype := multipartBody(t,
		map[string]string{"title": "", "text": "look https://example.com", "url": "https://example.com"},
		map[string]string{"photo.jpg": "jpegdata"})
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Post(ts.URL+"/share", ctype, body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d", res.StatusCode)
	}
	res, _ = http.Get(ts.URL + "/api/messages")
	var msgs []store.Message
	json.NewDecoder(res.Body).Decode(&msgs)
	if len(msgs) != 2 || msgs[0].Kind != store.KindFile || msgs[1].Body != "look https://example.com" {
		t.Fatalf("messages = %+v", msgs)
	}
}

func TestEventsStream(t *testing.T) {
	ts := newServer(t, ident.Dev{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/events", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	r := bufio.NewReader(res.Body)
	r.ReadString('\n') // retry line: the stream is subscribed

	http.Post(ts.URL+"/api/messages?as=phone", "application/json", strings.NewReader(`{"text":"live"}`))

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("stream ended: %v", err)
		}
		if strings.HasPrefix(line, "data: ") {
			var m store.Message
			json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &m)
			if m.Body != "live" || m.Device != "phone" {
				t.Fatalf("event = %+v", m)
			}
			return
		}
	}
}

type deny struct{}

func (deny) Identify(*http.Request) (store.Sender, error) { return store.Sender{}, ident.ErrForbidden }

func TestForbiddenDevice(t *testing.T) {
	ts := newServer(t, deny{})
	for _, path := range []string{"/", "/api/messages", "/api/events"} {
		res, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("%s: status = %d", path, res.StatusCode)
		}
	}
}

func TestCrossOriginPostRejected(t *testing.T) {
	ts := newServer(t, ident.Dev{})
	req, _ := http.NewRequest("POST", ts.URL+"/api/messages", strings.NewReader(`{"text":"pwned"}`))
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("Origin", "https://evil.example")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", res.StatusCode)
	}

	// The browser-initiated share target navigation must still work.
	body, ctype := multipartBody(t, map[string]string{"text": "shared"}, nil)
	req, _ = http.NewRequest("POST", ts.URL+"/share", body)
	req.Header.Set("Content-Type", ctype)
	req.Header.Set("Sec-Fetch-Site", "none")
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("share status = %d", res.StatusCode)
	}
}

func TestFormFieldBudget(t *testing.T) {
	ts := newServer(t, ident.Dev{})
	fields := map[string]string{}
	for i := range 3 {
		fields["junk"+strconv.Itoa(i)] = strings.Repeat("x", MaxTextLen/2)
	}
	body, ctype := multipartBody(t, fields, nil)
	res, err := http.Post(ts.URL+"/share", ctype, body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", res.StatusCode)
	}
}
