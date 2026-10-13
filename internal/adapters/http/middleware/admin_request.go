package middleware

import (
	"mime"
	"net/http"
	"net/url"
	"strings"
)

func RequireAdminHost(publicURL string) func(http.Handler) http.Handler {
	allowedHost, defaultPort := "", ""
	if u, err := url.Parse(publicURL); err == nil && u.Hostname() != "" && u.User == nil {
		switch u.Scheme {
		case "http":
			defaultPort = ":80"
		case "https":
			defaultPort = ":443"
		}
		if defaultPort != "" {
			allowedHost = strings.TrimSuffix(u.Host, defaultPort)
			if u.Port() != "" && ":"+u.Port() != defaultPort {
				defaultPort = ""
			}
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if allowedHost == "" || !strings.EqualFold(strings.TrimSuffix(r.Host, defaultPort), allowedHost) {
				http.Error(w, "unrecognized admin Host", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func RequireAdminJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requiresJSON := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch || r.ContentLength != 0 || len(r.TransferEncoding) != 0
		if requiresJSON {
			mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || mediaType != "application/json" || len(r.Header.Values("Content-Type")) != 1 {
				http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
