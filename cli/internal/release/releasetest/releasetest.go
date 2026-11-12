// Package releasetest provides a fake release server for tests.
package releasetest

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

// Server mimics the layout of https://github.com/OWNER/REPO/releases:
//
//	/releases/latest                  302 -> /releases/tag/<latest>, or /releases if none
//	/releases/latest/download/<file>  302 -> /releases/download/<latest>/<file>
//	/releases/download/<tag>/<file>   release assets
type Server struct {
	*httptest.Server

	mu        sync.Mutex
	latest    string
	files     map[string][]byte
	downloads int
}

// NewServer starts a server with no releases. Close it when done.
func NewServer() *Server {
	s := &Server{files: map[string][]byte{}}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	return s
}

// BaseURL is the releases URL to give a release.Client.
func (s *Server) BaseURL() string { return s.URL + "/releases" }

// AddRelease publishes tag with bin as the goos/goarch binary, plus a
// SHA256SUMS, and makes it the latest release.
func (s *Server) AddRelease(tag, goos, goarch string, bin []byte) {
	name := fmt.Sprintf("rembox-%s-%s", goos, goarch)
	sums := fmt.Sprintf("%x  %s\n", sha256.Sum256(bin), name)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[tag+"/"+name] = bin
	s.files[tag+"/SHA256SUMS"] = []byte(sums)
	s.latest = tag
}

// SetFile overrides a release asset, e.g. to serve a tampered binary.
func (s *Server) SetFile(tag, name string, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[tag+"/"+name] = body
}

// SetLatest changes the tag /latest redirects to ("" means no releases).
func (s *Server) SetLatest(tag string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latest = tag
}

// Downloads is how many binaries (assets other than SHA256SUMS) have been served.
func (s *Server) Downloads() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.downloads
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch p := r.URL.Path; {
	case p == "/releases/latest":
		if s.latest == "" {
			http.Redirect(w, r, "/releases", http.StatusFound)
		} else {
			http.Redirect(w, r, "/releases/tag/"+s.latest, http.StatusFound)
		}
	case strings.HasPrefix(p, "/releases/latest/download/") && s.latest != "":
		http.Redirect(w, r, "/releases/download/"+s.latest+"/"+strings.TrimPrefix(p, "/releases/latest/download/"), http.StatusFound)
	case strings.HasPrefix(p, "/releases/download/"):
		body, ok := s.files[strings.TrimPrefix(p, "/releases/download/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if !strings.HasSuffix(p, "/SHA256SUMS") {
			s.downloads++
		}
		w.Write(body)
	default:
		http.NotFound(w, r)
	}
}
