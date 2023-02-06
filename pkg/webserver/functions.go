package webserver

import (
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/gorilla/mux"
)

const (
	keyLength   = 36       // exact key length for a valid key.
	apiKey      = "apiKey" // used for map key internally.
	keyPosition = 5        // example: /api/v1/route/method/apikey
)

type responseWrapper struct {
	http.ResponseWriter
	statusCode int
}

func (r *responseWrapper) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func (s *server) countRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodDelete {
			s.exp.Add("Deletes", 1)
		}

		if req.Header.Get("X-Server") != "" {
			s.exp.Add("X-Server", 1)
		}

		s.exp.Add("Total", 1)
		wrap := &responseWrapper{ResponseWriter: resp, statusCode: http.StatusOK}
		next.ServeHTTP(wrap, req)
		s.exp.Add(fmt.Sprintf("Response %d %s", wrap.statusCode, http.StatusText(wrap.statusCode)), 1)
	})
}

// parseAPIKey sets a valid-lengh api key to a mux var.
// or returns a 401 if no key is found.
func (s *server) parseAPIKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		origURI := req.Header.Get("X-Original-URI")
		uri := strings.Split(origURI, "/")
		// This is for the log file.
		if len(uri) > keyPosition {
			req.Header.Set("x-uri", path.Dir(origURI))
		} else {
			req.Header.Set("x-uri", origURI)
		}

		key := req.Header.Get("X-API-Key")
		if len(key) != keyLength {
			if len(uri) > keyPosition {
				key = strings.Split(uri[keyPosition], "?")[0]
			}
		}

		req = mux.SetURLVars(req, map[string]string{apiKey: key})

		if len(key) != keyLength {
			s.exp.Add("Invalid Key", 1)
			s.noKeyReply(resp, req) // bad key, bail out.
		} else {
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

func maskAPIKey(key string) (string, int) {
	length := len(key)
	if length < 10 {
		return key, length
	}

	return key[:4] + "..." + key[length-2:], length
}
