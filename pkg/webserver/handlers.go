package webserver

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Notifiarr/mysql-auth-proxy/pkg/cache"
	"github.com/Notifiarr/mysql-auth-proxy/pkg/userinfo"
	"github.com/gorilla/mux"
)

func (s *server) handleGetKey(resp http.ResponseWriter, req *http.Request) {
	start := time.Now()

	userInfo, err := s.cache.GetUserInfo(req.Context(), mux.Vars(req)[apiKey])
	if err != nil {
		log.Printf("[ERROR] %v", err)
	}

	resp.Header().Set("X-Environment", userInfo.Environment)
	resp.Header().Set("X-Username", userInfo.Username)
	resp.Header().Set("X-UserID", userInfo.UserID)
	resp.Header().Set("Age", strconv.Itoa(int((time.Since(userInfo.Cached).Seconds()))))
	resp.Header().Set("X-Request-Time", time.Since(start).Round(time.Millisecond).String())

	if userInfo.UserID == userinfo.DefaultUserID && err == nil {
		s.noKeyReply(resp, req)
	} else {
		resp.WriteHeader(http.StatusOK)
	}
}

func (s *server) handleDelKey(resp http.ResponseWriter, req *http.Request) {
	start := time.Now()
	keys := strings.Split(mux.Vars(req)[apiKey], ",")
	infos := make([]*cache.UserInfo, len(keys))

	for idx, key := range keys {
		infos[idx] = s.cache.DelCacheKey(key)
		// Something is better than nothing.
		if infos[idx].UserID != userinfo.DefaultUserID {
			resp.Header().Set("X-UserID", infos[idx].UserID)
			resp.Header().Set("X-Username", infos[idx].Username)
		}
	}

	resp.Header().Set("X-Environment", "deleted")
	resp.Header().Set("Content-Type", "application/json")
	resp.Header().Set("Age", strconv.Itoa(len(infos)))
	resp.Header().Set("X-Request-Time", time.Since(start).Round(time.Millisecond).String())
	resp.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(resp).Encode(infos); err != nil {
		log.Printf("[ERROR] writing response: %v", err)
	}
}

// noKeyReply returns a 401.
func (s *server) noKeyReply(resp http.ResponseWriter, req *http.Request) {
	key, length := maskAPIKey(mux.Vars(req)[apiKey])
	resp.Header().Set("X-Key", key)
	resp.Header().Set("X-Length", strconv.Itoa(length))
	resp.WriteHeader(http.StatusUnauthorized)

	if _, err := resp.Write([]byte("invalid or no key provided")); err != nil {
		log.Printf("[ERROR] writing response: %v", err)
	}
}
