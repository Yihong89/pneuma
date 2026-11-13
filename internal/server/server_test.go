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

type fakeDevices []ident.Device

func (f fakeDevices) Devices(context.Context) ([]ident.Device, error) { return f, nil }

// tailnet is the set of devices the private-chat tests know about.
var tailnet = fakeDevices{
	{ID: ident.DevID("laptop"), Name: "laptop"},
	{ID: ident.DevID("mac-mini"), Name: "mac-mini"},
	{ID: ident.DevID("phone"), Name: "phone"},
}

func newServer(t *testing.T, id ident.Identifier) *httptest.Server {
	return newServerWith(t, id, nil)
}

func newServerWith(t *testing.T, id ident.Identifier, devices ident.DeviceLister) *httptest.Server {
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
		Devices:   devices,
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

	if res := do(t, "PUT", ts.URL+"/api/devices/pixel-phone/alias", `{"alias":"  mi   móvil "}`); res.StatusCode != http.StatusOK {
		t.Fatalf("alias = %d", res.StatusCode)
	}
	var got struct {
		Settings settings          `json:"settings"`
		Aliases  map[string]string `json:"aliases"`
	}
	json.NewDecoder(do(t, "GET", ts.URL+"/api/settings", "").Body).Decode(&got)
	if got.Settings.RetentionDays != 30 || got.Settings.MaxUpload != 2097152 || got.Aliases["pixel-phone"] != "mi móvil" {
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
	ts := newServerWith(t, ident.Dev{}, tailnet)
	if res := as(t, ts, "laptop", "POST", "/api/chats", `{"name":"x","members":["dev:stranger"]}`); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("chat with unknown device = %d", res.StatusCode)
	}
	res := as(t, ts, "laptop", "POST", "/api/chats", `{"name":"secret","members":["dev:mac-mini"]}`)
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
		{"POST", "/api/chats/" + id + "/members", `{"device":"dev:phone"}`},
		{"GET", fileURL, ""},
		{"DELETE", "/api/messages/" + strconv.FormatInt(files[0].ID, 10), ""},
	} {
		if res := as(t, ts, "phone", probe.method, probe.path, probe.body); res.StatusCode != http.StatusNotFound {
			t.Errorf("outsider %s %s = %d, want 404", probe.method, probe.path, res.StatusCode)
		}
	}

	// Members can add someone, who then gets in.
	if res := as(t, ts, "laptop", "POST", "/api/chats/"+id+"/members", `{"device":"dev:ghost"}`); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("add unknown device = %d", res.StatusCode)
	}
	if res := as(t, ts, "laptop", "POST", "/api/chats/"+id+"/members", `{"device":"dev:phone"}`); res.StatusCode != http.StatusOK {
		t.Fatalf("add member = %d", res.StatusCode)
	}
	if res := as(t, ts, "phone", "GET", fileURL, ""); res.StatusCode != http.StatusOK {
		t.Fatalf("new member download = %d", res.StatusCode)
	}
}

func TestPrivateEventsStayPrivate(t *testing.T) {
	ts := newServerWith(t, ident.Dev{}, tailnet)
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
	json.NewDecoder(as(t, ts, "laptop", "POST", "/api/chats", `{"name":"secret","members":["dev:mac-mini"]}`).Body).Decode(&c)
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

func TestQuoteMessage(t *testing.T) {
	ts := newServer(t, ident.Dev{})

	var first store.Message
	res := do(t, "POST", ts.URL+"/api/chats/1/messages", `{"text":"original"}`)
	json.NewDecoder(res.Body).Decode(&first)

	// Quoting an existing message round-trips its id.
	var reply store.Message
	res = do(t, "POST", ts.URL+"/api/chats/1/messages", `{"text":"citing you","reply_to":`+strconv.FormatInt(first.ID, 10)+`}`)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("reply = %d", res.StatusCode)
	}
	json.NewDecoder(res.Body).Decode(&reply)
	if reply.ReplyTo == nil || *reply.ReplyTo != first.ID {
		t.Fatalf("reply_to = %v, want %d", reply.ReplyTo, first.ID)
	}
	// It comes back on listing too.
	var msgs []store.Message
	json.NewDecoder(do(t, "GET", ts.URL+"/api/chats/1/messages", "").Body).Decode(&msgs)
	if msgs[len(msgs)-1].ReplyTo == nil || *msgs[len(msgs)-1].ReplyTo != first.ID {
		t.Fatalf("listed reply lost reply_to: %+v", msgs[len(msgs)-1])
	}

	// Quoting a missing message is a bad request, not a silent drop.
	if res := do(t, "POST", ts.URL+"/api/chats/1/messages", `{"text":"dangling","reply_to":999999}`); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("dangling quote = %d, want 400", res.StatusCode)
	}
	// Quoting across chats is refused: the original lives in chat 1.
	var other store.Chat
	json.NewDecoder(do(t, "POST", ts.URL+"/api/chats", `{"name":"other"}`).Body).Decode(&other)
	otherID := strconv.FormatInt(other.ID, 10)
	if res := do(t, "POST", ts.URL+"/api/chats/"+otherID+"/messages", `{"text":"wrong chat","reply_to":`+strconv.FormatInt(first.ID, 10)+`}`); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("cross-chat quote = %d, want 400", res.StatusCode)
	}
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

// fixedIdent always answers as one sender so a test can switch who a
// request comes from without a tailnet.
type fixedIdent struct{ sender store.Sender }

func (f *fixedIdent) Identify(*http.Request) (store.Sender, error) { return f.sender, nil }

func TestDeletionRights(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	who := &fixedIdent{sender: store.Sender{ID: "user:boss@x.com", User: "boss@x.com", Device: "mac"}}
	s := &Server{
		Store:     st,
		Hub:       hub.New(),
		Ident:     who,
		Web:       fstest.MapFS{"index.html": {Data: []byte("<h1>pneuma</h1>")}},
		MaxUpload: 1 << 20,
		// Deliberate mixed case: matching must be case insensitive.
		Deleters: []string{"Boss@x.com", "partner@x.com"},
	}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	canDelete := func() bool {
		var me struct {
			CanDelete bool `json:"can_delete"`
		}
		json.NewDecoder(do(t, "GET", ts.URL+"/api/me", "").Body).Decode(&me)
		return me.CanDelete
	}

	if !canDelete() {
		t.Fatal("a deleter should be told they can delete")
	}
	var m store.Message
	res := do(t, "POST", ts.URL+"/api/chats/1/messages", `{"text":"keep or drop"}`)
	json.NewDecoder(res.Body).Decode(&m)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("post = %d", res.StatusCode)
	}
	msgID := "/api/messages/" + strconv.FormatInt(m.ID, 10)

	// Someone off the list: still chats, never deletes.
	who.sender = store.Sender{ID: "user:kid@x.com", User: "kid@x.com", Device: "ipad"}
	if canDelete() {
		t.Fatal("a non-deleter should be told they cannot delete")
	}
	if res := do(t, "POST", ts.URL+"/api/chats/1/messages", `{"text":"hi"}`); res.StatusCode != http.StatusCreated {
		t.Fatalf("non-deleter post = %d, want 201", res.StatusCode)
	}
	if res := do(t, "DELETE", ts.URL+msgID, ""); res.StatusCode != http.StatusForbidden {
		t.Fatalf("non-deleter delete message = %d, want 403", res.StatusCode)
	}
	if res := do(t, "DELETE", ts.URL+"/api/chats/1", ""); res.StatusCode != http.StatusForbidden {
		t.Fatalf("non-deleter delete chat = %d, want 403", res.StatusCode)
	}
	if res := do(t, "PUT", ts.URL+"/api/devices/ipad/alias", `{"alias":"tablet"}`); res.StatusCode != http.StatusForbidden {
		t.Fatalf("non-deleter rename = %d, want 403", res.StatusCode)
	}

	// Back to the deleter: deletes and renames go through.
	who.sender = store.Sender{ID: "user:boss@x.com", User: "boss@x.com", Device: "mac"}
	if res := do(t, "PUT", ts.URL+"/api/devices/ipad/alias", `{"alias":"tablet"}`); res.StatusCode != http.StatusOK {
		t.Fatalf("deleter rename = %d, want 200", res.StatusCode)
	}
	if res := do(t, "DELETE", ts.URL+msgID, ""); res.StatusCode != http.StatusNoContent {
		t.Fatalf("deleter delete = %d, want 204", res.StatusCode)
	}
}
