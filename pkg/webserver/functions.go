// Package webserver contains the web server and related functions.
package webserver

import (
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/Notifiarr/mysql-auth-proxy/pkg/exp"
	"github.com/gorilla/mux"
)

const (
	keyLength = 36       // exact key length for a valid key.
	apiKey    = "apiKey" // used for map key internally.
	// website uses this position for the api key, e.g. /api/v1/route/method/{apikey} <-- 5.
	keyPosition = 5
)

// RefererPathForLog returns the path part of X-Original-Uri (no query string) truncated before the
// API key segment (keyPosition), using the same strings.Split(path, "/") rules as GetAPIKeyFromURIPath.
// If the path has fewer than keyPosition+1 segments, it returns the full path (still without query).
// When X-Original-Uri is missing, empty, or only a query string, it returns "".
func RefererPathForLog(header http.Header) string {
	pathPart, _, _ := strings.Cut(header.Get("X-Original-Uri"), "?")
	if pathPart == "" {
		return ""
	}

	var pos, segIdx int

	for seg := range strings.SplitSeq(pathPart, "/") {
		if segIdx == keyPosition {
			return strings.TrimSuffix(pathPart[:pos], "/")
		}

		pos += len(seg)
		if pos < len(pathPart) && pathPart[pos] == '/' {
			pos++
		}

		segIdx++
	}

	return pathPart
}

// ClientIPForLog returns the client IP for access logs (same rules as the former fixForwardedFor middleware).
func ClientIPForLog(req *http.Request) string {
	forwarded := req.Header.Get("X-Forwarded-For")
	if forwarded == "" {
		host, _, err := net.SplitHostPort(req.RemoteAddr)
		if err != nil {
			return strings.Trim(req.RemoteAddr, "[]")
		}

		return host
	}

	return strings.TrimSpace(strings.Split(forwarded, ",")[0])
}

// GetAPIKeyFromURIPath returns segment keyPosition of strings.Split(pathStr, "/") (without
// allocating the split slice). If that segment contains "?", only the part before it is returned.
// If pathStr has fewer than keyPosition+1 segments, it returns "".
func GetAPIKeyFromURIPath(pathStr string) string {
	segIdx := 0

	for seg := range strings.SplitSeq(pathStr, "/") {
		if segIdx == keyPosition {
			before, _, _ := strings.Cut(seg, "?")
			return before
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

// parseAPIKey sets a valid-lengh api key to a mux var.
// or returns a 401 if no key is found.
func (s *server) parseAPIKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		key := req.Header.Get("X-Api-Key")
		if len(key) != keyLength {
			key = GetAPIKeyFromURIPath(req.Header.Get("X-Original-Uri"))
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

func maskAPIKey(key string) (string, string) {
	const showKeyLength = 10

	length := len(key)
	if length < showKeyLength {
		return key, strconv.Itoa(length)
	}

	return key[:4] + "..." + key[length-2:], strconv.Itoa(length)
}
