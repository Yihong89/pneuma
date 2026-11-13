// Package server implements pneuma's HTTP API and serves the web app.
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ericpastorm/pneuma/internal/hub"
	"github.com/ericpastorm/pneuma/internal/ident"
	"github.com/ericpastorm/pneuma/internal/store"
)

// MaxTextLen bounds a single text message.
const MaxTextLen = 1 << 20

// Server wires the store, the event hub and the identity source together.
type Server struct {
	Store     *store.Store
	Hub       *hub.Hub
	Ident     ident.Identifier
	Web       fs.FS // static web app
	MaxUpload int64 // default per-request limit in bytes; overridable in settings
	// Devices lists the tailnet's devices; nil in dev mode, where only
	// devices with pneuma open are shown.
	Devices ident.DeviceLister
}

// MaxChatName bounds a chat's name.
const MaxChatName = 80

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/me", s.handleMe)
	mux.HandleFunc("GET /api/devices", s.handleDevices)
	mux.HandleFunc("PUT /api/devices/{name}/alias", s.handleAlias)
	mux.HandleFunc("GET /api/settings", s.handleSettings)
	mux.HandleFunc("PATCH /api/settings", s.handleUpdateSettings)
	mux.HandleFunc("GET /api/chats", s.handleChats)
	mux.HandleFunc("POST /api/chats", s.handleCreateChat)
	mux.HandleFunc("PATCH /api/chats/{chat}", s.handleRenameChat)
	mux.HandleFunc("DELETE /api/chats/{chat}", s.handleDeleteChat)
	mux.HandleFunc("POST /api/chats/{chat}/members", s.handleAddMember)
	mux.HandleFunc("GET /api/chats/{chat}/messages", s.handleList)
	mux.HandleFunc("POST /api/chats/{chat}/messages", s.handleText)
	mux.HandleFunc("POST /api/chats/{chat}/files", s.handleFiles)
	mux.HandleFunc("GET /api/chats/{chat}/files", s.handleListFiles)
	mux.HandleFunc("DELETE /api/messages/{id}", s.handleDelete)
	mux.HandleFunc("GET /api/events", s.handleEvents)
	mux.HandleFunc("GET /f/{id}/{name}", s.handleDownload)
	mux.HandleFunc("POST /share", s.handleShare)
	mux.Handle("GET /", s.static())
	// Any tailnet device's browser carries its identity, so a hostile page
	// open on one of them must not be able to post or delete on its behalf.
	return s.authenticate(http.NewCrossOriginProtection().Handler(mux))
}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sender, err := s.Ident.Identify(r)
		if err != nil {
			if errors.Is(err, ident.ErrForbidden) {
				http.Error(w, "this device is not allowed to use pneuma", http.StatusForbidden)
				return
			}
			log.Printf("identify %s: %v", r.RemoteAddr, err)
			http.Error(w, "could not identify device", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(ident.WithSender(r.Context(), sender)))
	})
}

func (s *Server) static() http.Handler {
	files := http.FileServerFS(s.Web)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always revalidate so a redeploy shows up immediately; the service
		// worker handles offline caching.
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"sender":     ident.SenderFrom(r.Context()),
		"max_upload": s.maxUpload(r.Context()),
	})
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	c, ok := s.visibleChat(w, r)
	if !ok {
		return
	}
	chat := c.ID
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	msgs, err := s.Store.List(r.Context(), chat, before, limit)
	if err != nil {
		serverError(w, err)
		return
	}
	if msgs == nil {
		msgs = []*store.Message{}
	}
	writeJSON(w, http.StatusOK, msgs)
}

func (s *Server) handleText(w http.ResponseWriter, r *http.Request) {
	c, ok := s.visibleChat(w, r)
	if !ok {
		return
	}
	chat := c.ID
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxTextLen+1024)).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		http.Error(w, "empty message", http.StatusBadRequest)
		return
	}
	if len(req.Text) > MaxTextLen {
		http.Error(w, "message too long", http.StatusRequestEntityTooLarge)
		return
	}
	m, err := s.Store.AddText(r.Context(), chat, ident.SenderFrom(r.Context()), req.Text)
	if err != nil {
		storeError(w, r, err)
		return
	}
	s.publishMessage(c, m)
	writeJSON(w, http.StatusCreated, m)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	m, err := s.Store.Get(r.Context(), id)
	var c *store.Chat
	if err == nil {
		c, err = s.chatFor(r, m.ChatID)
	}
	if err == nil {
		err = s.Store.Delete(r.Context(), id)
	}
	if err != nil {
		storeError(w, r, err)
		return
	}
	s.publishTo(c.Members, "delete", map[string]int64{"id": id, "chat_id": m.ChatID})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	c, ok := s.visibleChat(w, r)
	if !ok {
		return
	}
	chat := c.ID
	msgs, err := s.Store.Files(r.Context(), chat)
	if err != nil {
		serverError(w, err)
		return
	}
	if msgs == nil {
		msgs = []*store.Message{}
	}
	writeJSON(w, http.StatusOK, msgs)
}

// handleFiles accepts a multipart body with one or more "file" parts and
// streams each straight to disk.
func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	c, ok := s.visibleChat(w, r)
	if !ok {
		return
	}
	msgs, _, err := s.receiveMultipart(w, r, c)
	if err != nil {
		uploadError(w, err)
		return
	}
	if len(msgs) == 0 {
		http.Error(w, "no files", http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, msgs)
}

// handleShare is the Web Share Target endpoint: Android's share sheet posts
// title/text/url fields and files here.
//
// Shares land in the most recently active chat.
func (s *Server) handleShare(w http.ResponseWriter, r *http.Request) {
	chats, err := s.visibleChats(r)
	if err != nil {
		serverError(w, err)
		return
	}
	if len(chats) == 0 {
		http.Error(w, "no chat to share into", http.StatusConflict)
		return
	}
	c := chats[0]
	chat := c.ID
	_, fields, err := s.receiveMultipart(w, r, c)
	if err != nil {
		uploadError(w, err)
		return
	}
	var parts []string
	for _, k := range []string{"title", "text", "url"} {
		if v := strings.TrimSpace(fields[k]); v != "" && !containedIn(parts, v) {
			parts = append(parts, v)
		}
	}
	if text := strings.Join(parts, "\n"); text != "" {
		m, err := s.Store.AddText(r.Context(), chat, ident.SenderFrom(r.Context()), text)
		if err != nil {
			serverError(w, err)
			return
		}
		s.publishMessage(c, m)
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// containedIn reports whether v already appears inside one of xs; Android
// often sends the same URL as both "text" and "url".
func containedIn(xs []string, v string) bool {
	for _, x := range xs {
		if strings.Contains(x, v) {
			return true
		}
	}
	return false
}

// receiveMultipart stores every file part as a message and returns the
// (small) non-file form fields.
func (s *Server) receiveMultipart(w http.ResponseWriter, r *http.Request, c *store.Chat) ([]*store.Message, map[string]string, error) {
	chat := c.ID
	limit := s.maxUpload(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, limit+1<<20)
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, nil, badRequest{err}
	}
	sender := ident.SenderFrom(r.Context())
	fields := map[string]string{}
	fieldBudget := int64(MaxTextLen) // shared by all non-file fields
	var msgs []*store.Message
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return msgs, fields, err
		}
		if part.FileName() == "" {
			b, err := io.ReadAll(io.LimitReader(part, fieldBudget+1))
			part.Close()
			if err != nil {
				return msgs, fields, err
			}
			if fieldBudget -= int64(len(b)); fieldBudget < 0 {
				return msgs, fields, store.ErrTooLarge
			}
			switch name := part.FormName(); name {
			case "title", "text", "url":
				fields[name] = string(b)
			}
			continue
		}
		name := cleanName(part.FileName())
		ctype := part.Header.Get("Content-Type")
		if ctype == "" || ctype == "application/octet-stream" {
			if t := mime.TypeByExtension(filepath.Ext(name)); t != "" {
				ctype = t
			}
		}
		m, err := s.Store.AddFile(r.Context(), chat, sender, name, ctype, part, limit)
		part.Close()
		if err != nil {
			return msgs, fields, err
		}
		s.publishMessage(c, m)
		msgs = append(msgs, m)
	}
	return msgs, fields, nil
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	chat, err := s.Store.FileChat(r.Context(), r.PathValue("id"))
	if err == nil {
		_, err = s.chatFor(r, chat)
	}
	if err != nil {
		storeError(w, r, err)
		return
	}
	f, fh, err := s.Store.OpenFile(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		serverError(w, err)
		return
	}
	defer fh.Close()

	disposition := "attachment"
	if inlineSafe(f.Mime) && r.URL.Query().Get("dl") == "" {
		disposition = "inline"
	}
	h := w.Header()
	h.Set("Content-Type", f.Mime)
	h.Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": f.Name}))
	h.Set("X-Content-Type-Options", "nosniff")
	// Uploaded content must never run script in our origin.
	h.Set("Content-Security-Policy", "sandbox; default-src 'none'; img-src 'self'; media-src 'self'; style-src 'unsafe-inline'")
	h.Set("Cache-Control", "private, max-age=31536000, immutable")
	// Files are immutable, so the upload time is a fine modtime.
	http.ServeContent(w, r, f.Name, time.Time{}, fh)
}

// inlineSafe reports whether a type can be displayed in the browser without
// risk of executing content (no HTML, no SVG).
func inlineSafe(ctype string) bool {
	t, _, _ := mime.ParseMediaType(ctype)
	switch {
	case t == "image/svg+xml":
		return false
	case strings.HasPrefix(t, "image/"), strings.HasPrefix(t, "video/"), strings.HasPrefix(t, "audio/"):
		return true
	case t == "application/pdf", t == "text/plain":
		return true
	}
	return false
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	me := ident.SenderFrom(r.Context())
	events, unsubscribe, joined := s.Hub.Subscribe(me.ID)
	defer func() {
		if unsubscribe() {
			s.publishPresence()
		}
	}()
	if joined {
		s.publishPresence()
	}

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "retry: 2000\n\n")
	flusher.Flush()

	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
		case e, ok := <-events:
			if !ok {
				return // dropped for being slow; the client reconnects and resyncs
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Type, e.Data)
		}
		flusher.Flush()
	}
}

func (s *Server) publishMessage(c *store.Chat, m *store.Message) {
	s.publishTo(c.Members, "message", m)
}

func (s *Server) publishPresence() {
	s.publish("presence", map[string][]string{"active": s.Hub.Active()})
}

func (s *Server) publish(typ string, v any) { s.publishTo(nil, typ, v) }

// publishTo sends an event only to the given devices (everyone if empty).
func (s *Server) publishTo(audience []string, typ string, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("marshal %s event: %v", typ, err)
		return
	}
	s.Hub.Publish(hub.Event{Type: typ, Data: data, Audience: audience})
}

// chatFor loads a chat and checks the requesting device may see it. Chats
// it can't see are reported as not found, so their existence doesn't leak.
func (s *Server) chatFor(r *http.Request, id int64) (*store.Chat, error) {
	c, err := s.Store.Chat(r.Context(), id)
	if err != nil {
		return nil, err
	}
	if !c.Visible(ident.SenderFrom(r.Context()).ID) {
		return nil, store.ErrNotFound
	}
	return c, nil
}

func (s *Server) visibleChat(w http.ResponseWriter, r *http.Request) (*store.Chat, bool) {
	id, ok := chatID(w, r)
	if !ok {
		return nil, false
	}
	c, err := s.chatFor(r, id)
	if err != nil {
		storeError(w, r, err)
		return nil, false
	}
	return c, true
}

func (s *Server) visibleChats(r *http.Request) ([]*store.Chat, error) {
	all, err := s.Store.Chats(r.Context())
	if err != nil {
		return nil, err
	}
	me := ident.SenderFrom(r.Context()).ID
	out := []*store.Chat{}
	for _, c := range all {
		if c.Visible(me) {
			out = append(out, c)
		}
	}
	return out, nil
}

// ---- chats ----

func (s *Server) handleChats(w http.ResponseWriter, r *http.Request) {
	chats, err := s.visibleChats(r)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, chats)
}

func (s *Server) handleCreateChat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string   `json:"name"`
		Members []string `json:"members"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	name, ok := cleanChatName(req.Name)
	if !ok {
		http.Error(w, "invalid chat name", http.StatusBadRequest)
		return
	}
	if len(req.Members) > 0 {
		for _, id := range req.Members {
			if !s.knownDevice(r, id) {
				http.Error(w, "unknown device", http.StatusBadRequest)
				return
			}
		}
		// A private chat always includes whoever creates it.
		req.Members = append(req.Members, ident.SenderFrom(r.Context()).ID)
	}
	c, err := s.Store.CreateChat(r.Context(), name, req.Members...)
	if err != nil {
		serverError(w, err)
		return
	}
	s.publishTo(c.Members, "chat", c)
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) handleAddMember(w http.ResponseWriter, r *http.Request) {
	c, ok := s.visibleChat(w, r)
	if !ok {
		return
	}
	var req struct {
		Device string `json:"device"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil || strings.TrimSpace(req.Device) == "" {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if len(c.Members) == 0 {
		http.Error(w, "chat is already open to every device", http.StatusBadRequest)
		return
	}
	if !s.knownDevice(r, req.Device) {
		http.Error(w, "unknown device", http.StatusBadRequest)
		return
	}
	c, err := s.Store.AddMember(r.Context(), c.ID, req.Device)
	if err != nil {
		storeError(w, r, err)
		return
	}
	s.publishTo(c.Members, "chat", c)
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleRenameChat(w http.ResponseWriter, r *http.Request) {
	c, ok := s.visibleChat(w, r)
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	name, ok := cleanChatName(req.Name)
	if !ok {
		http.Error(w, "invalid chat name", http.StatusBadRequest)
		return
	}
	c, err := s.Store.RenameChat(r.Context(), c.ID, name)
	if err != nil {
		storeError(w, r, err)
		return
	}
	s.publishTo(c.Members, "chat", c)
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleDeleteChat(w http.ResponseWriter, r *http.Request) {
	c, ok := s.visibleChat(w, r)
	if !ok {
		return
	}
	fresh, err := s.Store.DeleteChat(r.Context(), c.ID)
	if err != nil {
		storeError(w, r, err)
		return
	}
	s.publishTo(c.Members, "chat_delete", map[string]int64{"id": c.ID})
	if fresh != nil {
		s.publish("chat", fresh)
	}
	w.WriteHeader(http.StatusNoContent)
}

func chatID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("chat"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return 0, false
	}
	return id, true
}

func cleanChatName(name string) (string, bool) {
	name = strings.Join(strings.Fields(name), " ")
	return name, name != "" && len([]rune(name)) <= MaxChatName
}

// ---- devices ----

// knownDevice reports whether the account may use pneuma right now: a
// current tailnet user passing the access rules, or the requester
// themselves. In dev mode, anyone with pneuma open counts.
func (s *Server) knownDevice(r *http.Request, want string) bool {
	if want == "" {
		return false
	}
	if want == ident.SenderFrom(r.Context()).ID {
		return true
	}
	if s.Devices == nil {
		for _, id := range s.Hub.Active() {
			if id == want {
				return true
			}
		}
		return false
	}
	devices, err := s.Devices.Devices(r.Context())
	if err != nil {
		log.Printf("list devices: %v", err)
		return false
	}
	for _, d := range devices {
		if d.ID == want {
			return true
		}
	}
	return false
}

type deviceStatus struct {
	ident.Device
	Active bool `json:"active"` // has pneuma open right now
}

func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	var devices []ident.Device
	if s.Devices != nil {
		var err error
		if devices, err = s.Devices.Devices(r.Context()); err != nil {
			serverError(w, err)
			return
		}
	}
	active := map[string]bool{}
	for _, id := range s.Hub.Active() {
		active[id] = true
	}
	out := []deviceStatus{}
	for _, d := range devices {
		out = append(out, deviceStatus{Device: d, Active: active[d.ID]})
		delete(active, d.ID)
	}
	// Someone with pneuma open may be missing from the list (dev mode's
	// fake logins, a peer the lister couldn't resolve); they are online by
	// definition, so they still get a row.
	me := ident.SenderFrom(r.Context())
	orphans := make([]string, 0, len(active))
	for id := range active {
		orphans = append(orphans, id)
	}
	slices.Sort(orphans)
	for _, id := range orphans {
		name, os := id, ""
		if dev := ident.DevName(id); dev != "" {
			name = dev
		}
		if id == me.ID {
			name, os = me.Device, me.OS
		}
		out = append(out, deviceStatus{Device: ident.Device{ID: id, Name: name, OS: os, Online: true}, Active: true})
	}
	writeJSON(w, http.StatusOK, out)
}

// cleanName strips any directory components a client may send.
func cleanName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Base(name)
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." || name == "/" {
		return "file"
	}
	return name
}

type badRequest struct{ error }

// storeError maps store errors to HTTP responses.
func storeError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	serverError(w, err)
}

func uploadError(w http.ResponseWriter, err error) {
	var maxErr *http.MaxBytesError
	switch {
	case errors.Is(err, store.ErrTooLarge), errors.As(err, &maxErr):
		http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
	case errors.As(err, new(badRequest)):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, store.ErrNotFound):
		http.Error(w, "chat not found", http.StatusNotFound)
	default:
		serverError(w, err)
	}
}

func serverError(w http.ResponseWriter, err error) {
	log.Printf("error: %v", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
