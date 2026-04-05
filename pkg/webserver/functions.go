// Package webserver contains the web server and related functions.
package webserver

import (
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/Notifiarr/mysql-auth-proxy/pkg/exp"
	"github.com/gorilla/mux"
)

const (
	keyLength   = 36       // exact key length for a valid key.
	apiKey      = "apiKey" // used for map key internally.
	keyPosition = 5        // example: /api/v1/route/method/apikey
)

// pathSegment returns the idx-th element of strings.Split(pathStr, "/") using strings.SplitSeq
// so the full split slice is not allocated.
func pathSegment(pathStr string, idx int) string {
	segIdx := 0

	for seg := range strings.SplitSeq(pathStr, "/") {
		if segIdx == idx {
			before, _, found := strings.Cut(seg, "?")
			if found {
				return before
			}

			return seg
		}

		segIdx++
	}

	return ""
}

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
		s.metrics.HTTPRequests.WithLabelValues(exp.HTTPEventTotal).Inc()

		if req.Method == http.MethodDelete {
			s.metrics.HTTPRequests.WithLabelValues(exp.HTTPEventDelete).Inc()
		}

		if req.Header.Get("X-Server") != "" {
			s.metrics.HTTPRequests.WithLabelValues(exp.HTTPEventXServer).Inc()
		}

		wrap := &responseWrapper{ResponseWriter: resp, statusCode: http.StatusOK}
		next.ServeHTTP(wrap, req)

		s.metrics.HTTPResponse.WithLabelValues(strconv.Itoa(wrap.statusCode)).Inc()
	})
}

// fixRequestURI sets a special header that we can log without an API key. That is all.
func (s *server) fixRequestURI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		origURI := req.Header.Get("X-Original-Uri")
		if uri := strings.Split(origURI, "/"); len(uri) > keyPosition {
			req.Header.Set("Referer", path.Dir(origURI))
		} else if origURI != "" {
			req.Header.Set("Referer", origURI)
		}

		next.ServeHTTP(resp, req)
	})
}

// parseAPIKey sets a valid-lengh api key to a mux var.
// or returns a 401 if no key is found.
func (s *server) parseAPIKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		key := req.Header.Get("X-Api-Key")
		if len(key) != keyLength {
			key = pathSegment(req.Header.Get("X-Original-Uri"), keyPosition)
		}

		pooled := s.apiKeyVarsPool.Get()

		urlVars, ok := pooled.(map[string]string)
		if !ok {
			urlVars = make(map[string]string, 1)
		}

		urlVars[apiKey] = key
		req = mux.SetURLVars(req, urlVars)

		defer func() {
			delete(urlVars, apiKey)
			s.apiKeyVarsPool.Put(urlVars)
		}()

		if len(key) != keyLength {
			s.metrics.HTTPRequests.WithLabelValues(exp.HTTPEventInvalidKey).Inc()
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
		forwarded := req.Header.Get("X-Forwarded-For")
		if forwarded == "" {
			host, _, err := net.SplitHostPort(req.RemoteAddr)
			if err != nil {
				req.Header.Set("X-Forwarded-For", strings.Trim(req.RemoteAddr, "[]"))
			} else {
				req.Header.Set("X-Forwarded-For", host)
			}
		} else {
			req.Header.Set("X-Forwarded-For", strings.TrimSpace(strings.Split(forwarded, ",")[0]))
		}

		next.ServeHTTP(resp, req)
	})
}

func maskAPIKey(key string) (string, int) {
	const showKeyLength = 10

	length := len(key)
	if length < showKeyLength {
		return key, length
	}

	return key[:4] + "..." + key[length-2:], length
}
