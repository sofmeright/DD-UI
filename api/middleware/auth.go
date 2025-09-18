package middleware

import (
	"context"
	"encoding/json"
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
		common.DebugLog("AUTH MIDDLEWARE: Checking auth for %s %s", r.Method, r.URL.Path)
		
		if common.SessionManager == nil {
			common.ErrorLog("AUTH MIDDLEWARE: SessionManager is nil!")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "error",
				"message": "Authentication system not configured",
			})
			return
		}
		
		common.DebugLog("AUTH MIDDLEWARE: SessionManager exists, getting user from context")
		u, ok := common.SessionManager.Get(r.Context(), "user").(User)
		exp := common.SessionManager.GetInt64(r.Context(), "exp")
		
		common.DebugLog("AUTH MIDDLEWARE: User found: %v, Exp: %d, Current time: %d", ok, exp, time.Now().Unix())
		
		if !ok || time.Now().Unix() > exp {
			common.WarnLog("AUTH MIDDLEWARE: Access denied - user: %v, expired: %v", ok, time.Now().Unix() > exp)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "error",
				"message": "Session expired or invalid. Please log in again.",
			})
			return
		}
		
		common.DebugLog("AUTH MIDDLEWARE: Access granted for user: %s", u.Email)
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