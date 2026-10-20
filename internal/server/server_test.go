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
	res, err := http.Post(ts.URL+"/api/chats/1/messages?as=mac-mini", "application/json", strings.NewReader(`{"text":"hola"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d", res.StatusCode)
	}

	res, err = http.Get(ts.URL + "/api/chats/1/messages")
	if err != nil {
		t.Fatal(err)
	}
	var msgs []store.Message
	json.NewDecoder(res.Body).Decode(&msgs)
	if len(msgs) != 1 || msgs[0].Body != "hola" || msgs[0].Device != "mac-mini" || msgs[0].OS != "macos" {
		t.Fatalf("messages = %+v", msgs)
	}

	res, _ = http.Post(ts.URL+"/api/chats/1/messages", "application/json", strings.NewReader(`{"text":"   "}`))
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty message status = %d", res.StatusCode)
	}
}

func TestUploadDownloadDelete(t *testing.T) {
	ts := newServer(t, ident.Dev{})
	body, ctype := multipartBody(t, nil, map[string]string{"a.txt": "alpha", "../../evil.svg": "<svg onload=alert(1)>"})
	res, err := http.Post(ts.URL+"/api/chats/1/files", ctype, body)
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
	res, err := http.Post(ts.URL+"/api/chats/1/files", ctype, body)
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
	res, _ = http.Get(ts.URL + "/api/chats/1/messages")
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

	http.Post(ts.URL+"/api/chats/1/messages?as=phone", "application/json", strings.NewReader(`{"text":"live"}`))

	event := ""
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("stream ended: %v", err)
		}
		if strings.HasPrefix(line, "event: ") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event: "))
		}
		if strings.HasPrefix(line, "data: ") && event == "message" {
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
	for _, path := range []string{"/", "/api/chats", "/api/chats/1/messages", "/api/events", "/api/devices"} {
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
	req, _ := http.NewRequest("POST", ts.URL+"/api/chats/1/messages", strings.NewReader(`{"text":"pwned"}`))
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

func do(t *testing.T, method, url, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestChatsAPI(t *testing.T) {
	ts := newServer(t, ident.Dev{})
	res := do(t, "POST", ts.URL+"/api/chats", `{"name":"  side   project "}`)
	var c store.Chat
	json.NewDecoder(res.Body).Decode(&c)
	if res.StatusCode != http.StatusCreated || c.Name != "side project" {
		t.Fatalf("create = %d %+v", res.StatusCode, c)
	}
	if res := do(t, "POST", ts.URL+"/api/chats", `{"name":""}`); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty name = %d", res.StatusCode)
	}

	id := strconv.FormatInt(c.ID, 10)
	do(t, "POST", ts.URL+"/api/chats/"+id+"/messages", `{"text":"only here"}`)
	if res := do(t, "POST", ts.URL+"/api/chats/999/messages", `{"text":"x"}`); res.StatusCode != http.StatusNotFound {
		t.Fatalf("missing chat = %d", res.StatusCode)
	}

	var msgs []store.Message
	json.NewDecoder(do(t, "GET", ts.URL+"/api/chats/1/messages", "").Body).Decode(&msgs)
	if len(msgs) != 0 {
		t.Fatalf("message leaked into general: %+v", msgs)
	}

	if res := do(t, "PATCH", ts.URL+"/api/chats/"+id, `{"name":"renamed"}`); res.StatusCode != http.StatusOK {
		t.Fatalf("rename = %d", res.StatusCode)
	}
	if res := do(t, "DELETE", ts.URL+"/api/chats/"+id, ""); res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete = %d", res.StatusCode)
	}
	var chats []store.Chat
	json.NewDecoder(do(t, "GET", ts.URL+"/api/chats", "").Body).Decode(&chats)
	if len(chats) != 1 || chats[0].Name != "general" {
		t.Fatalf("chats after delete = %+v", chats)
	}
}

func TestDevicesShowsActive(t *testing.T) {
	ts := newServer(t, ident.Dev{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/events?as=mac-mini", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	bufio.NewReader(res.Body).ReadString('\n')

	var devs []deviceStatus
	json.NewDecoder(do(t, "GET", ts.URL+"/api/devices", "").Body).Decode(&devs)
	if len(devs) != 1 || devs[0].Name != "mac-mini" || !devs[0].Active {
		t.Fatalf("devices = %+v", devs)
	}
}

func TestSettingsAndAliases(t *testing.T) {
	ts := newServer(t, ident.Dev{})
	if res := do(t, "PATCH", ts.URL+"/api/settings", `{"max_upload": 10}`); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("tiny max_upload accepted: %d", res.StatusCode)
	}
	if res := do(t, "PATCH", ts.URL+"/api/settings", `{"max_upload": 2097152, "retention_days": 30}`); res.StatusCode != http.StatusOK {
		t.Fatalf("update = %d", res.StatusCode)
	}
	// the new limit applies to uploads
	body, ctype := multipartBody(t, nil, map[string]string{"big.bin": strings.Repeat("x", 3<<20)})
	if res, _ := http.Post(ts.URL+"/api/chats/1/files", ctype, body); res.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("upload over new limit = %d", res.StatusCode)
	}

	if res := do(t, "PUT", ts.URL+"/api/devices/nothing-phone-3a/alias", `{"alias":"  mi   móvil "}`); res.StatusCode != http.StatusOK {
		t.Fatalf("alias = %d", res.StatusCode)
	}
	var got struct {
		Settings settings          `json:"settings"`
		Aliases  map[string]string `json:"aliases"`
	}
	json.NewDecoder(do(t, "GET", ts.URL+"/api/settings", "").Body).Decode(&got)
	if got.Settings.RetentionDays != 30 || got.Settings.MaxUpload != 2097152 || got.Aliases["nothing-phone-3a"] != "mi móvil" {
		t.Fatalf("settings = %+v", got)
	}
}

func TestListFiles(t *testing.T) {
	ts := newServer(t, ident.Dev{})
	do(t, "POST", ts.URL+"/api/chats/1/messages", `{"text":"not a file"}`)
	body, ctype := multipartBody(t, nil, map[string]string{"a.png": "png", "b.pdf": "pdf"})
	http.Post(ts.URL+"/api/chats/1/files", ctype, body)
	var files []store.Message
	json.NewDecoder(do(t, "GET", ts.URL+"/api/chats/1/files", "").Body).Decode(&files)
	if len(files) != 2 || files[0].ID < files[1].ID || files[0].File == nil {
		t.Fatalf("files = %+v", files)
	}
}

// as returns a client that poses as device in dev mode.
func as(t *testing.T, ts *httptest.Server, device, method, path, body string) *http.Response {
	t.Helper()
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return do(t, method, ts.URL+path+sep+"as="+device, body)
}

func TestPrivateChats(t *testing.T) {
	ts := newServer(t, ident.Dev{})
	res := as(t, ts, "laptop", "POST", "/api/chats", `{"name":"secret","members":["mac-mini"]}`)
	var c store.Chat
	json.NewDecoder(res.Body).Decode(&c)
	if res.StatusCode != http.StatusCreated || len(c.Members) != 2 {
		t.Fatalf("create = %d %+v", res.StatusCode, c)
	}
	id := strconv.FormatInt(c.ID, 10)
	as(t, ts, "mac-mini", "POST", "/api/chats/"+id+"/messages", `{"text":"only us"}`)
	body, ctype := multipartBody(t, nil, map[string]string{"plan.txt": "secret plan"})
	req, _ := http.NewRequest("POST", ts.URL+"/api/chats/"+id+"/files?as=laptop", body)
	req.Header.Set("Content-Type", ctype)
	res, _ = http.DefaultClient.Do(req)
	var files []store.Message
	json.NewDecoder(res.Body).Decode(&files)
	fileURL := "/f/" + files[0].File.ID + "/plan.txt"

	// An outsider sees nothing and can't touch anything.
	var chats []store.Chat
	json.NewDecoder(as(t, ts, "phone", "GET", "/api/chats", "").Body).Decode(&chats)
	for _, ch := range chats {
		if ch.ID == c.ID {
			t.Fatalf("outsider can list the private chat")
		}
	}
	for _, probe := range []struct{ method, path, body string }{
		{"GET", "/api/chats/" + id + "/messages", ""},
		{"GET", "/api/chats/" + id + "/files", ""},
		{"POST", "/api/chats/" + id + "/messages", `{"text":"hi"}`},
		{"PATCH", "/api/chats/" + id, `{"name":"mine now"}`},
		{"DELETE", "/api/chats/" + id, ""},
		{"POST", "/api/chats/" + id + "/members", `{"device":"phone"}`},
		{"GET", fileURL, ""},
		{"DELETE", "/api/messages/" + strconv.FormatInt(files[0].ID, 10), ""},
	} {
		if res := as(t, ts, "phone", probe.method, probe.path, probe.body); res.StatusCode != http.StatusNotFound {
			t.Errorf("outsider %s %s = %d, want 404", probe.method, probe.path, res.StatusCode)
		}
	}

	// Members can add someone, who then gets in.
	if res := as(t, ts, "laptop", "POST", "/api/chats/"+id+"/members", `{"device":"phone"}`); res.StatusCode != http.StatusOK {
		t.Fatalf("add member = %d", res.StatusCode)
	}
	if res := as(t, ts, "phone", "GET", fileURL, ""); res.StatusCode != http.StatusOK {
		t.Fatalf("new member download = %d", res.StatusCode)
	}
}

func TestPrivateEventsStayPrivate(t *testing.T) {
	ts := newServer(t, ident.Dev{})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/events?as=phone", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	lines := make(chan string, 100)
	go func() {
		r := bufio.NewReader(res.Body)
		for {
			l, err := r.ReadString('\n')
			if err != nil {
				close(lines)
				return
			}
			lines <- l
		}
	}()
	<-lines // subscribed

	var c store.Chat
	json.NewDecoder(as(t, ts, "laptop", "POST", "/api/chats", `{"name":"secret","members":["mac-mini"]}`).Body).Decode(&c)
	as(t, ts, "laptop", "POST", "/api/chats/"+strconv.FormatInt(c.ID, 10)+"/messages", `{"text":"psst"}`)
	as(t, ts, "laptop", "POST", "/api/chats/1/messages", `{"text":"public"}`)

	for l := range lines {
		if strings.Contains(l, "psst") || strings.Contains(l, "secret") {
			t.Fatalf("outsider received private event: %s", l)
		}
		if strings.Contains(l, "public") {
			return
		}
	}
	t.Fatal("stream ended before the public message")
}

func TestFileRetentionDefault(t *testing.T) {
	ts := newServer(t, ident.Dev{})
	var got struct {
		Settings settings `json:"settings"`
	}
	json.NewDecoder(do(t, "GET", ts.URL+"/api/settings", "").Body).Decode(&got)
	if got.Settings.FileRetentionDays != 30 || got.Settings.RetentionDays != 0 {
		t.Fatalf("defaults = %+v", got.Settings)
	}
}
