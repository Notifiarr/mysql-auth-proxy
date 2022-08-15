package webserver

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Notifiarr/mysql-auth-proxy/pkg/userinfo"
	"golift.io/cache"
)

/* This file contains the delete handlers. Used by automation. */

func (s *server) handleDelSrv(resp http.ResponseWriter, req *http.Request) {
	start := time.Now()
	user := userinfo.DefaultUser()

	item := s.servers.Get(req.Header.Get("X-Server"))

	defer s.servers.Delete(req.Header.Get("X-Server"))

	// These headers are mostly for logs.
	if user != nil && user.UserID != userinfo.DefaultUserID {
		resp.Header().Set("X-UserID", user.UserID)
		resp.Header().Set("X-Username", user.Username)
	}

	resp.Header().Set("X-Environment", "deleted")
	resp.Header().Set("Content-Type", "application/json")
	resp.Header().Set("Age", "1")
	resp.Header().Set("X-Request-Time", time.Since(start).Round(time.Millisecond).String())
	resp.WriteHeader(http.StatusOK)

	if item == nil {
		if err := json.NewEncoder(resp).Encode(map[string]any{"exists": false}); err != nil {
			s.Printf("[ERROR] writing response: %v", err)
		}
	} else if err := json.NewEncoder(resp).Encode(item.Data); err != nil {
		s.Printf("[ERROR] writing response: %v", err)
	}

}

func (s *server) handleDelKey(resp http.ResponseWriter, req *http.Request) {
	start := time.Now()
	keys := strings.Split(req.Header.Get("X-API-Keys"), ",")
	infos := make([]*cache.Item, len(keys))
	user := userinfo.DefaultUser()

	for idx, key := range keys {
		infos[idx] = s.users.Get(key)
		defer s.users.Delete(key)

		if infos[idx] != nil && infos[idx].Data != nil {
			user, _ = infos[idx].Data.(*userinfo.UserInfo)
		}

		// Something is better than nothing.
		if user != nil && user.UserID != userinfo.DefaultUserID {
			resp.Header().Set("X-UserID", user.UserID)
			resp.Header().Set("X-Username", user.Username)
		}
	}

	resp.Header().Set("X-Environment", "deleted")
	resp.Header().Set("Content-Type", "application/json")
	resp.Header().Set("Age", strconv.Itoa(len(infos)))
	resp.Header().Set("X-Request-Time", time.Since(start).Round(time.Millisecond).String())
	resp.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(resp).Encode(infos); err != nil {
		s.Printf("[ERROR] writing response: %v", err)
	}
}
