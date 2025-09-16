package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/gorilla/sessions"
)

// User represents an authenticated user
type User struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Pic   string `json:"picture,omitempty"`
}

// SessionStore interface defines the methods needed for session management
type SessionStore interface {
	Get(r *http.Request, name string) (*sessions.Session, error)
}

// AuthMiddleware provides authentication middleware functionality
type AuthMiddleware struct {
	store       SessionStore
	sessionName string
}

// NewAuthMiddleware creates a new authentication middleware instance
func NewAuthMiddleware(store SessionStore, sessionName string) *AuthMiddleware {
	return &AuthMiddleware{
		store:       store,
		sessionName: sessionName,
	}
}

type ctxKey string

const UserKey ctxKey = "dd-ui.user"

// RequireAuth is a middleware that requires authentication
func (am *AuthMiddleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, _ := am.store.Get(r, am.sessionName)
		u, ok := sess.Values["user"].(User)
		exp, _ := sess.Values["exp"].(int64)
		if !ok || time.Now().Unix() > exp {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), UserKey, u)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// CurrentUser extracts the current user from the request context
func CurrentUser(ctx context.Context) User {
	if v := ctx.Value(UserKey); v != nil {
		if u, ok := v.(User); ok {
			return u
		}
	}
	return User{}
}