// Command pneuma is a private chat and file drop for the devices on your
// tailnet. It joins the tailnet as its own node, so every request already
// tells it which device is talking: there are no accounts or passwords.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"tailscale.com/tsnet"

	"github.com/ericpastorm/pneuma/internal/hub"
	"github.com/ericpastorm/pneuma/internal/ident"
	"github.com/ericpastorm/pneuma/internal/server"
	"github.com/ericpastorm/pneuma/internal/store"
	"github.com/ericpastorm/pneuma/web"
)

func main() {
	var (
		dev      = flag.Bool("dev", false, "serve on localhost without Tailscale, for development")
		devAddr  = flag.String("dev-addr", "localhost:8080", "listen address in -dev mode")
		hostname = flag.String("hostname", env("PNEUMA_HOSTNAME", "pneuma"), "node name on the tailnet")
		dataDir  = flag.String("data", env("PNEUMA_DATA_DIR", defaultDataDir()), "directory for messages, files and tailnet state")
		maxSize  = flag.String("max-upload", env("PNEUMA_MAX_UPLOAD", "4GB"), "maximum upload size per request")
		allowTag = flag.Bool("allow-tagged", envBool("PNEUMA_ALLOW_TAGGED"), "allow tagged nodes (servers) to use pneuma")
		users    = flag.String("allowed-users", os.Getenv("PNEUMA_ALLOWED_USERS"), "comma-separated tailnet logins allowed in (default: everyone)")
	)
	flag.Parse()

	maxUpload, err := parseSize(*maxSize)
	if err != nil {
		log.Fatalf("invalid -max-upload: %v", err)
	}
	st, err := store.Open(*dataDir)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	srv := &server.Server{Store: st, Hub: hub.New(), Web: web.FS, MaxUpload: maxUpload}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *dev {
		srv.Ident = ident.Dev{}
		ln, err := net.Listen("tcp", *devAddr)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("pneuma (dev) on http://%s — add ?as=<device> to pretend to be another machine", *devAddr)
		serve(ctx, srv.Handler(), ln)
		return
	}

	ts := &tsnet.Server{
		Hostname: *hostname,
		Dir:      filepath.Join(*dataDir, "tailscale"),
		AuthKey:  os.Getenv("TS_AUTHKEY"),
		UserLogf: log.Printf,
	}
	defer ts.Close()
	status, err := ts.Up(ctx)
	if err != nil {
		log.Fatalf("join tailnet: %v", err)
	}
	lc, err := ts.LocalClient()
	if err != nil {
		log.Fatal(err)
	}
	srv.Ident = &ident.Tailscale{Client: lc, AllowTagged: *allowTag, AllowedUsers: splitList(*users)}

	ln, err := ts.ListenTLS("tcp", ":443")
	if err != nil {
		log.Fatal(err)
	}
	httpLn, err := ts.Listen("tcp", ":80")
	if err != nil {
		log.Fatal(err)
	}
	domain := strings.TrimSuffix(status.Self.DNSName, ".")
	go http.Serve(httpLn, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://"+domain+r.URL.RequestURI(), http.StatusMovedPermanently)
	}))
	log.Printf("pneuma is up at https://%s", domain)
	serve(ctx, srv.Handler(), ln)
}

func serve(ctx context.Context, h http.Handler, ln net.Listener) {
	hs := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		hs.Shutdown(shutdown)
	}()
	if err := hs.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func defaultDataDir() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "pneuma")
	}
	return "pneuma-data"
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string) bool {
	b, _ := strconv.ParseBool(os.Getenv(key))
	return b
}

func splitList(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// parseSize parses sizes like "500MB", "4GB" or a plain byte count.
func parseSize(s string) (int64, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	mult := int64(1)
	for _, u := range []struct {
		suffix string
		mult   int64
	}{{"TB", 1 << 40}, {"GB", 1 << 30}, {"MB", 1 << 20}, {"KB", 1 << 10}, {"B", 1}} {
		if strings.HasSuffix(s, u.suffix) {
			s, mult = strings.TrimSpace(strings.TrimSuffix(s, u.suffix)), u.mult
			break
		}
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%q is not a size", s)
	}
	return int64(n * float64(mult)), nil
}
