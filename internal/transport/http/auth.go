package http

import (
	"crypto/subtle"
	"net/http"

	"github.com/SamuelFan1/Axis/internal/config"
)

func adminAuthMiddleware(authCfg config.AuthConfig, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || !secureEqual(username, authCfg.AdminUsername) || !secureEqual(password, authCfg.AdminPassword) {
			w.Header().Set("WWW-Authenticate", `Basic realm="`+authCfg.Realm+`"`)
			writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
				"error": "unauthorized",
			})
			return
		}
		next(w, r)
	}
}

func nodeTokenMiddleware(authCfg config.AuthConfig, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Axis-Node-Token")
		if !secureEqual(token, authCfg.NodeSharedToken) {
			writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
				"error": "unauthorized",
			})
			return
		}
		next(w, r)
	}
}

func secureEqual(left, right string) bool {
	if len(left) != len(right) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}
