package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Settings shared by every device, editable from the web app.
const (
	keyRetention = "retention_days" // 0 keeps everything
	keyMaxUpload = "max_upload"     // bytes

	MaxAlias     = 40
	maxRetention = 3650
	minUpload    = 1 << 20
	maxUpload    = 1 << 40
)

type settings struct {
	RetentionDays int64 `json:"retention_days"`
	MaxUpload     int64 `json:"max_upload"`
}

func (s *Server) settings(ctx context.Context) (settings, error) {
	var st settings
	var err error
	if st.RetentionDays, err = s.Store.SettingInt(ctx, keyRetention, 0); err != nil {
		return st, err
	}
	st.MaxUpload, err = s.Store.SettingInt(ctx, keyMaxUpload, s.MaxUpload)
	return st, err
}

func (s *Server) maxUpload(ctx context.Context) int64 {
	n, err := s.Store.SettingInt(ctx, keyMaxUpload, s.MaxUpload)
	if err != nil {
		log.Printf("read max_upload: %v", err)
		return s.MaxUpload
	}
	return n
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	st, err := s.settings(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	usage, err := s.Store.Usage(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	aliases, err := s.Store.Aliases(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": st, "usage": usage, "aliases": aliases})
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RetentionDays *int64 `json:"retention_days"`
		MaxUpload     *int64 `json:"max_upload"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	if v := req.RetentionDays; v != nil {
		if *v < 0 || *v > maxRetention {
			http.Error(w, "retention_days out of range", http.StatusBadRequest)
			return
		}
		if err := s.Store.SetSetting(ctx, keyRetention, strconv.FormatInt(*v, 10)); err != nil {
			serverError(w, err)
			return
		}
	}
	if v := req.MaxUpload; v != nil {
		if *v < minUpload || *v > maxUpload {
			http.Error(w, "max_upload out of range", http.StatusBadRequest)
			return
		}
		if err := s.Store.SetSetting(ctx, keyMaxUpload, strconv.FormatInt(*v, 10)); err != nil {
			serverError(w, err)
			return
		}
	}
	st, err := s.settings(ctx)
	if err != nil {
		serverError(w, err)
		return
	}
	s.publish("settings", st)
	if req.RetentionDays != nil {
		s.applyRetention(ctx)
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleAlias(w http.ResponseWriter, r *http.Request) {
	device := r.PathValue("name")
	var req struct {
		Alias string `json:"alias"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil || device == "" {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	alias := strings.Join(strings.Fields(req.Alias), " ")
	if len([]rune(alias)) > MaxAlias {
		http.Error(w, "alias too long", http.StatusBadRequest)
		return
	}
	if alias == device {
		alias = ""
	}
	if err := s.Store.SetAlias(r.Context(), device, alias); err != nil {
		serverError(w, err)
		return
	}
	aliases, err := s.Store.Aliases(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	s.publish("aliases", aliases)
	writeJSON(w, http.StatusOK, aliases)
}

// RunRetention deletes expired messages now and then every hour until ctx
// is done.
func (s *Server) RunRetention(ctx context.Context) {
	s.applyRetention(ctx)
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.applyRetention(ctx)
		}
	}
}

func (s *Server) applyRetention(ctx context.Context) {
	days, err := s.Store.SettingInt(ctx, keyRetention, 0)
	if err != nil || days <= 0 {
		return
	}
	n, err := s.Store.DeleteOlderThan(ctx, time.Now().AddDate(0, 0, -int(days)))
	if err != nil {
		log.Printf("retention: %v", err)
		return
	}
	if n > 0 {
		log.Printf("retention: removed %d messages older than %d days", n, days)
		s.publish("resync", map[string]int64{"removed": n})
	}
}
