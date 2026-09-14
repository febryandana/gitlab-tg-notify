package webhook

import (
	"crypto/subtle"
	"net/http"
)

// verifyToken checks the X-Gitlab-Token header against the configured
// webhook secret using a constant-time comparison, per PRD §13.5.
func verifyToken(r *http.Request, secret string) bool {
	got := r.Header.Get("X-Gitlab-Token")
	if got == "" || secret == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(secret)) == 1
}
