package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"incipit/internal/appdb"
)

const (
	sessionCookie = "incipit_session"
	csrfCookie    = "incipit_csrf"
	csrfHeader    = "X-CSRF-Token"
	sessionTTL    = 30 * 24 * time.Hour
)

type csrfKeyT int

const csrfKey csrfKeyT = 0

// recoverer turns panics into 500s instead of crashing the server.
func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic", "error", rec, "stack", string(debug.Stack()))
				writeError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// requestLogger emits a structured log line per request.
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		slog.Info("request",
			"method", r.Method, "path", r.URL.Path, "status", sw.status,
			"bytes", sw.bytes, "dur", time.Since(start).String(), "ip", clientIP(r))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(c int) { w.status = c; w.ResponseWriter.WriteHeader(c) }
func (w *statusWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// requireAuth authenticates via a trusted reverse proxy (if configured) or the
// session cookie, attaching the user and a CSRF token to the context.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, seed := s.authenticate(r)
		if user == nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		token := s.csrfToken(seed)
		s.ensureCSRFCookie(w, r, token)

		ctx := withUser(r.Context(), user)
		ctx = context.WithValue(ctx, csrfKey, token)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// authenticate resolves the request's user and returns a stable CSRF seed.
func (s *Server) authenticate(r *http.Request) (*appdb.User, string) {
	if s.proxyCfg.Enabled {
		if u, err := s.auth.ResolveProxyUser(r.Context(), s.proxyCfg, r.Header.Get); err == nil && u != nil {
			return u, "proxy:" + u.Username
		}
	}
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		if sess, err := s.store.GetSession(r.Context(), c.Value); err == nil {
			if u, err := s.store.GetUser(r.Context(), sess.UserID); err == nil {
				return u, "session:" + sess.ID
			}
		}
	}
	// HTTP Basic fallback so OPDS/API clients (which hold no session cookie) can
	// authenticate. Rate-limited per IP to blunt brute-force attempts.
	if username, password, ok := r.BasicAuth(); ok && s.limiter.Allow(clientIP(r)) {
		if u, err := s.auth.Login(r.Context(), username, password); err == nil {
			return u, "basic:" + u.Username
		}
	}
	return nil, ""
}

// csrfProtect enforces a double-submit CSRF token on unsafe methods.
func (s *Server) csrfProtect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		want, _ := r.Context().Value(csrfKey).(string)
		got := r.Header.Get(csrfHeader)
		if want == "" || subtle.ConstantTimeCompare([]byte(want), []byte(got)) != 1 {
			writeError(w, http.StatusForbidden, "invalid CSRF token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireAdmin gates admin-only routes.
func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u := currentUser(r); u == nil || !u.IsAdmin {
			writeError(w, http.StatusForbidden, "admin privileges required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// basicAuth authenticates OPDS clients via HTTP Basic credentials.
func (s *Server) basicAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if ok {
			if u, err := s.auth.Login(r.Context(), username, password); err == nil {
				next.ServeHTTP(w, r.WithContext(withUser(r.Context(), u)))
				return
			}
		}
		// Allow a reverse proxy to authenticate OPDS too.
		if s.proxyCfg.Enabled {
			if u, err := s.auth.ResolveProxyUser(r.Context(), s.proxyCfg, r.Header.Get); err == nil && u != nil {
				next.ServeHTTP(w, r.WithContext(withUser(r.Context(), u)))
				return
			}
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="Incipit OPDS"`)
		writeError(w, http.StatusUnauthorized, "authentication required")
	})
}

// csrfToken derives a stable token from a seed using the session secret.
func (s *Server) csrfToken(seed string) string {
	if seed == "" {
		return ""
	}
	mac := hmac.New(sha256.New, s.cfg.SessionSecret)
	mac.Write([]byte(seed))
	return hex.EncodeToString(mac.Sum(nil))[:32]
}

func (s *Server) ensureCSRFCookie(w http.ResponseWriter, r *http.Request, token string) {
	if c, err := r.Cookie(csrfCookie); err == nil && c.Value == token {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: false, // readable by the SPA to echo in the CSRF header
		Secure:   s.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   s.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: name == sessionCookie, Secure: s.cfg.SecureCookies, SameSite: http.SameSiteLaxMode,
	})
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// errForbidden / errNotFound helpers for handlers.
var errSessionToken = errors.New("could not create session token")
