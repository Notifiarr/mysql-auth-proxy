// Package webserver contains the web server and related functions.
package webserver

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/Notifiarr/mysql-auth-proxy/pkg/exp"
)

const (
	keyLength   = 36 // exact key length for a valid key.
	keyPosition = 5  // website uses this position for the api key, e.g. /api/v1/route/method/{apikey} <-- 5.
)

type parsedAPIKeyCtxKey struct{}

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

// apiKeyFromRequest returns the parsed API key set by parseAPIKey (empty if unset).
func apiKeyFromRequest(req *http.Request) string {
	v, _ := req.Context().Value(parsedAPIKeyCtxKey{}).(string)
	return v
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

		if getHeader(req.Header, "X-Server") != "" {
			s.metrics.HTTPRequests.WithLabelValues(exp.HTTPEventXServer).Inc()
		}

		wrap := &responseWrapper{ResponseWriter: resp, statusCode: http.StatusOK}
		next.ServeHTTP(wrap, req)

		s.metrics.HTTPResponse.WithLabelValues(strconv.Itoa(wrap.statusCode)).Inc()
	})
}

// parseAPIKey attaches the parsed API key to req's context for downstream handlers,
// or returns a 401 if no key is found.
func (s *server) parseAPIKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		key := getHeader(req.Header, "X-Api-Key")
		if len(key) != keyLength {
			key = GetAPIKeyFromURIPath(getHeader(req.Header, "X-Original-Uri"))
		}

		req = req.WithContext(context.WithValue(req.Context(), parsedAPIKeyCtxKey{}, key))

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

// getHeader returns the first value for an http.Header field. key must already be in
// canonical form (http.CanonicalHeaderKey). Unlike Header.Get it does not allocate
// or re-canonicalize key on each call.
func getHeader(headers http.Header, key string) string {
	if v := headers[key]; len(v) > 0 {
		return v[0]
	}

	return ""
}
