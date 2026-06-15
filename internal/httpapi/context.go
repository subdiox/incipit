package httpapi

import (
	"context"
	"net/http"

	"incipit/internal/appdb"
)

type ctxKey int

const userKey ctxKey = iota

func withUser(ctx context.Context, u *appdb.User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

// currentUser returns the authenticated user from the request context, or nil.
func currentUser(r *http.Request) *appdb.User {
	u, _ := r.Context().Value(userKey).(*appdb.User)
	return u
}
