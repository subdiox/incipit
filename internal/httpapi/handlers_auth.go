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

// handleSetupStatus reports whether first-run admin setup is needed, and
// whether that setup must also collect the Calibre library path (i.e. it was
// not provided via INCIPIT_LIBRARY).
func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.CountUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "count users")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{
		"needsSetup":   n == 0,
		"needsLibrary": !s.libraryConfigured(),
	})
}

type credentials struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	LibraryPath string `json:"libraryPath"`
}

// handleSetup creates the first admin account (only when no users exist) and,
// when the library has not been configured yet, opens and persists the Calibre
// library at the supplied path.
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

	// Configure the library first so a bad path fails before any account is
	// created (the admin can simply retry).
	if !s.libraryConfigured() {
		path := strings.TrimSpace(c.LibraryPath)
		if path == "" {
			writeError(w, http.StatusBadRequest, "library path is required")
			return
		}
		if err := s.openLibrary(path); err != nil {
			writeError(w, http.StatusBadRequest, "cannot open library at that path: "+err.Error())
			return
		}
		if err := s.store.SetSetting(r.Context(), LibraryPathKey, path); err != nil {
			writeError(w, http.StatusInternalServerError, "save library path")
			return
		}
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

// updateMeBody holds the self-service account fields a user may change.
type updateMeBody struct {
	Language *string `json:"language"`
	PageSize *int    `json:"pageSize"`
}

// handleUpdateMe lets the authenticated user change their own preferences
// (currently just the UI language). Admin-only fields stay in handleUpdateUser.
func (s *Server) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	cur := currentUser(r)
	if cur == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var body updateMeBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Language != nil {
		lang := strings.ToLower(strings.TrimSpace(*body.Language))
		if lang != "en" && lang != "ja" {
			writeError(w, http.StatusBadRequest, "unsupported language")
			return
		}
		if err := s.store.SetUserLanguage(r.Context(), cur.ID, lang); err != nil {
			writeError(w, http.StatusInternalServerError, "update language")
			return
		}
	}
	if body.PageSize != nil {
		ps := *body.PageSize
		if ps < appdb.MinPageSize || ps > appdb.MaxPageSize {
			writeError(w, http.StatusBadRequest, "page size out of range")
			return
		}
		if err := s.store.SetUserPageSize(r.Context(), cur.ID, ps); err != nil {
			writeError(w, http.StatusInternalServerError, "update page size")
			return
		}
	}
	u, err := s.store.GetUser(r.Context(), cur.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load user")
		return
	}
	writeJSON(w, http.StatusOK, u)
}
