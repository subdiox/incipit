package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"incipit/internal/appdb"
	"incipit/internal/auth"
)

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", errSessionToken
	}
	return hex.EncodeToString(b), nil
}

// handleSetupStatus reports whether first-run admin setup is needed.
func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.CountUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "count users")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"needsSetup": n == 0})
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// handleSetup creates the first admin account (only when no users exist).
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.CountUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "count users")
		return
	}
	if n > 0 {
		writeError(w, http.StatusForbidden, "setup already completed")
		return
	}
	var c credentials
	if err := decodeJSON(r, &c); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if strings.TrimSpace(c.Username) == "" || len(c.Password) < 8 {
		writeError(w, http.StatusBadRequest, "username required and password must be at least 8 characters")
		return
	}
	hash, err := auth.HashPassword(c.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "hash password")
		return
	}
	u, err := s.store.CreateUser(r.Context(), appdb.User{
		Username:     strings.TrimSpace(c.Username),
		PasswordHash: hash,
		IsAdmin:      true,
		Source:       appdb.SourceLocal,
		CanDownload:  true,
		CanUpload:    true,
		CanEdit:      true,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create user")
		return
	}
	if err := s.startSession(w, r, u); err != nil {
		writeError(w, http.StatusInternalServerError, "start session")
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

// handleLogin authenticates a user and starts a session.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.limiter.Allow(clientIP(r)) {
		writeError(w, http.StatusTooManyRequests, "too many login attempts, try again later")
		return
	}
	var c credentials
	if err := decodeJSON(r, &c); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	u, err := s.auth.Login(r.Context(), c.Username, c.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, "invalid username or password")
			return
		}
		writeError(w, http.StatusInternalServerError, "login failed")
		return
	}
	if err := s.startSession(w, r, u); err != nil {
		writeError(w, http.StatusInternalServerError, "start session")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// startSession creates a session, sets the session cookie and a matching CSRF
// cookie so the SPA can immediately make authenticated mutations.
func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u *appdb.User) error {
	token, err := generateToken()
	if err != nil {
		return err
	}
	expires := time.Now().Add(sessionTTL)
	if err := s.store.CreateSession(r.Context(), appdb.Session{
		ID: token, UserID: u.ID, CreatedAt: time.Now(), ExpiresAt: expires,
	}); err != nil {
		return err
	}
	s.setSessionCookie(w, token, expires)
	s.ensureCSRFCookie(w, r, s.csrfToken("session:"+token))
	return nil
}

// handleLogout ends the session and clears cookies.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		_ = s.store.DeleteSession(r.Context(), c.Value)
	}
	s.clearCookie(w, sessionCookie)
	s.clearCookie(w, csrfCookie)
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

// handleMe returns the authenticated user.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, currentUser(r))
}
