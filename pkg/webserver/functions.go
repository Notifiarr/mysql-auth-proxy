package webserver

import (
	"net/http"
	"strings"

	"github.com/gorilla/mux"
)

const (
	keyLength   = 36       // exact key length for a valid key.
	apiKey      = "apiKey" // used for map key internally.
	keyPosition = 5        // example: /api/v1/route/method/apikey
)

// parseKey attempts to get the key from either the url (mux vars), or two different headers.
func parseKey(req *http.Request) string {
	if key := mux.Vars(req)["key"]; len(key) == keyLength {
		return key // deprecate this.
	} else if key = req.Header.Get("X-API-Key"); len(key) == keyLength {
		return key
	} else if uri := strings.Split(req.Header.Get("X-Original-URI"), "/"); len(uri) > keyPosition {
		return strings.Split(uri[keyPosition], "?")[0]
	}

	return ""
}

// parseAPIKey sets a valid-lengh api key to a mux var.
// or returns a 401 if no key is found.
func parseAPIKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		if key := parseKey(req); len(key) != keyLength {
			noKeyReply(resp, req) // bad key, bail out.
		} else {
			mux.SetURLVars(req, map[string]string{apiKey: key})
			next.ServeHTTP(resp, req)
		}
	})
}

// fixForwardedFor sets the X-Forwarded-For header to the client IP.
// This does not validate the upstream IP. Do not expose this to the Internet.
func fixForwardedFor(next http.Handler) http.Handler {
	return http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		if x := req.Header.Get("X-Forwarded-For"); x == "" {
			ip := strings.Trim(req.RemoteAddr[:strings.LastIndex(req.RemoteAddr, ":")], "[]")
			req.Header.Set("X-Forwarded-For", ip)
		} else {
			req.Header.Set("X-Forwarded-For", strings.TrimSpace(strings.Split(x, ",")[0]))
		}

		next.ServeHTTP(resp, req)
	})
}
