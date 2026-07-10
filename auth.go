package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
)

// basicAuth wraps h with HTTP Basic Auth using a constant-time comparison.
// The browser sends the Authorization header on websocket upgrades too, so a
// single middleware protects both the UI and /ws.
func basicAuth(user, pass string, h http.Handler) http.Handler {
	userHash := sha256.Sum256([]byte(user))
	passHash := sha256.Sum256([]byte(pass))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if ok {
			uh := sha256.Sum256([]byte(u))
			ph := sha256.Sum256([]byte(p))
			if subtle.ConstantTimeCompare(uh[:], userHash[:]) == 1 &&
				subtle.ConstantTimeCompare(ph[:], passHash[:]) == 1 {
				h.ServeHTTP(w, r)
				return
			}
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="webterminal", charset="UTF-8"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}
