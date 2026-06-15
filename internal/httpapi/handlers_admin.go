package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"incipit/internal/appdb"
	"incipit/internal/auth"
)

// handleListUsers returns all users (admin only).
func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list users")
		return
	}
	if users == nil {
		users = []appdb.User{}
	}
	writeJSON(w, http.StatusOK, users)
}

type createUserBody struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	IsAdmin     bool   `json:"isAdmin"`
	CanDownload bool   `json:"canDownload"`
	CanUpload   bool   `json:"canUpload"`
	CanEdit     bool   `json:"canEdit"`
}

// handleCreateUser creates a local user (admin only).
func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var body createUserBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if strings.TrimSpace(body.Username) == "" || len(body.Password) < 8 {
		writeError(w, http.StatusBadRequest, "username required and password must be at least 8 characters")
		return
	}
	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "hash password")
		return
	}
	u, err := s.store.CreateUser(r.Context(), appdb.User{
		Username:     strings.TrimSpace(body.Username),
		PasswordHash: hash,
		IsAdmin:      body.IsAdmin,
		Source:       appdb.SourceLocal,
		CanDownload:  body.CanDownload,
		CanUpload:    body.CanUpload,
		CanEdit:      body.CanEdit,
	})
	if err != nil {
		writeError(w, http.StatusConflict, "username already exists")
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

type updateUserBody struct {
	Password    *string `json:"password"`
	IsAdmin     *bool   `json:"isAdmin"`
	CanDownload *bool   `json:"canDownload"`
	CanUpload   *bool   `json:"canUpload"`
	CanEdit     *bool   `json:"canEdit"`
}

// handleUpdateUser updates a user's permissions or password (admin only).
func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	u, err := s.store.GetUser(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	var body updateUserBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Password != nil {
		if len(*body.Password) < 8 {
			writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
			return
		}
		hash, err := auth.HashPassword(*body.Password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "hash password")
			return
		}
		u.PasswordHash = hash
	}
	if body.IsAdmin != nil {
		u.IsAdmin = *body.IsAdmin
	}
	if body.CanDownload != nil {
		u.CanDownload = *body.CanDownload
	}
	if body.CanUpload != nil {
		u.CanUpload = *body.CanUpload
	}
	if body.CanEdit != nil {
		u.CanEdit = *body.CanEdit
	}
	if err := s.store.UpdateUser(r.Context(), u); err != nil {
		writeError(w, http.StatusInternalServerError, "update user")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// handleDeleteUser removes a user (admin only); an admin cannot delete itself.
func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if cur := currentUser(r); cur != nil && cur.ID == id {
		writeError(w, http.StatusBadRequest, "cannot delete your own account")
		return
	}
	if err := s.store.DeleteUser(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "delete user")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
