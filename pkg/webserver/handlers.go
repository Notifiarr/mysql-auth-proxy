package webserver

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
)

func (s *server) handleKey(resp http.ResponseWriter, req *http.Request) {
	start := time.Now()

	userInfo, err := s.cache.GetUserInfo(req.Context(), mux.Vars(req)[apiKey])
	if err != nil {
		log.Printf("[ERROR] %v", err)
	}

	resp.Header().Add("X-Environment", userInfo.Environment)
	resp.Header().Add("X-Username", userInfo.Username)
	resp.Header().Add("X-UserID", userInfo.UserID)
	resp.Header().Add("Age", strconv.Itoa(int((time.Since(userInfo.Cached).Seconds()))))
	resp.Header().Add("X-Request-Time", time.Since(start).Round(time.Millisecond).String())
	resp.WriteHeader(http.StatusOK)
}

func (s *server) handleDelKey(resp http.ResponseWriter, req *http.Request) {
	start := time.Now()

	userInfo, err := s.cache.DelCacheKey(mux.Vars(req)[apiKey])
	if err != nil {
		log.Printf("[ERROR] deleting cached key: %v", err)
	}

	resp.Header().Add("Content-Type", "application/json")
	resp.Header().Add("X-Request-Time", time.Since(start).Round(time.Millisecond).String())
	resp.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(resp).Encode(userInfo); err != nil {
		log.Printf("[ERROR] writing response: %v", err)
	}
}

// noKeyReply returns a 401.
func noKeyReply(resp http.ResponseWriter, _ *http.Request) {
	resp.WriteHeader(http.StatusUnauthorized)

	if _, err := resp.Write([]byte("no key provided")); err != nil {
		log.Printf("[ERROR] writing response: %v", err)
	}
}
