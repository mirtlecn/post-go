package core

import "net/http"

// IsAuthenticated checks Authorization: Bearer <SECRET_KEY>.
func IsAuthenticated(r *http.Request, secret string) bool {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		auth = r.Header.Get("authorization")
	}
	const prefix = "Bearer "
	if len(auth) <= len(prefix) || auth[:len(prefix)] != prefix {
		return false
	}
	return auth[len(prefix):] == secret
}
