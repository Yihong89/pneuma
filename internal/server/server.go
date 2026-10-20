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
	MaxUpload int64 // per request, in bytes
}

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/me", s.handleMe)
	mux.HandleFunc("GET /api/messages", s.handleList)
	mux.HandleFunc("POST /api/messages", s.handleText)
	mux.HandleFunc("DELETE /api/messages/{id}", s.handleDelete)
	mux.HandleFunc("POST /api/files", s.handleFiles)
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
		"max_upload": s.MaxUpload,
	})
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	msgs, err := s.Store.List(r.Context(), before, limit)
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
	m, err := s.Store.AddText(r.Context(), ident.SenderFrom(r.Context()), req.Text)
	if err != nil {
		serverError(w, err)
		return
	}
	s.publishMessage(m)
	writeJSON(w, http.StatusCreated, m)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := s.Store.Delete(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		serverError(w, err)
		return
	}
	data, _ := json.Marshal(map[string]int64{"id": id})
	s.Hub.Publish(hub.Event{Type: "delete", Data: data})
	w.WriteHeader(http.StatusNoContent)
}

// handleFiles accepts a multipart body with one or more "file" parts and
// streams each straight to disk.
func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	msgs, _, err := s.receiveMultipart(w, r)
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
func (s *Server) handleShare(w http.ResponseWriter, r *http.Request) {
	_, fields, err := s.receiveMultipart(w, r)
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
		m, err := s.Store.AddText(r.Context(), ident.SenderFrom(r.Context()), text)
		if err != nil {
			serverError(w, err)
			return
		}
		s.publishMessage(m)
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
func (s *Server) receiveMultipart(w http.ResponseWriter, r *http.Request) ([]*store.Message, map[string]string, error) {
	r.Body = http.MaxBytesReader(w, r.Body, s.MaxUpload+1<<20)
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
		m, err := s.Store.AddFile(r.Context(), sender, name, ctype, part, s.MaxUpload)
		part.Close()
		if err != nil {
			return msgs, fields, err
		}
		s.publishMessage(m)
		msgs = append(msgs, m)
	}
	return msgs, fields, nil
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
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
	events, unsubscribe := s.Hub.Subscribe()
	defer unsubscribe()

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

func (s *Server) publishMessage(m *store.Message) {
	data, err := json.Marshal(m)
	if err != nil {
		log.Printf("marshal message: %v", err)
		return
	}
	s.Hub.Publish(hub.Event{Type: "message", Data: data})
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

func uploadError(w http.ResponseWriter, err error) {
	var maxErr *http.MaxBytesError
	switch {
	case errors.Is(err, store.ErrTooLarge), errors.As(err, &maxErr):
		http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
	case errors.As(err, new(badRequest)):
		http.Error(w, err.Error(), http.StatusBadRequest)
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
