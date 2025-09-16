package middleware

import (
	"context"
	"net/http"
	"time"

	"dd-ui/common"
)

// User represents an authenticated user
type User struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Pic   string `json:"picture,omitempty"`
}

// Context key type
type ctxKey string

const UserKey ctxKey = "ddui.user"

// RequireAuth is a middleware that requires authentication
// It checks the session for a valid user and expiration time
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if common.SessionManager == nil {
			http.Error(w, "auth not configured", http.StatusInternalServerError)
			return
		}
		
		u, ok := common.SessionManager.Get(r.Context(), "user").(User)
		exp := common.SessionManager.GetInt64(r.Context(), "exp")
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

// GetUserEmail extracts the user's email from context (for audit logs)
func GetUserEmail(ctx context.Context) string {
	u := CurrentUser(ctx)
	if u.Email != "" {
		return u.Email
	}
	if u.Name != "" {
		return u.Name
	}
	if u.Sub != "" {
		return u.Sub
	}
	return "anonymous"
}