package webserver

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Notifiarr/mysql-auth-proxy/pkg/cache"
	"github.com/Notifiarr/mysql-auth-proxy/pkg/userinfo"
	"github.com/gorilla/mux"
)

type keyReq struct {
	key   string
	cache *cache.Item
	get   func(context.Context, string) (*userinfo.UserInfo, error)
	save  func(string, interface{}, bool) bool
}

func (s *server) handleServer(resp http.ResponseWriter, req *http.Request) {
	s.handleGetAny(resp, req, &keyReq{
		key:   req.Header.Get("X-Server"),
		cache: s.servers.Get(req.Header.Get("X-Server")),
		get:   s.ui.GetServer,
		save:  s.servers.Save,
	})
}

func (s *server) handleGetKey(resp http.ResponseWriter, req *http.Request) {
	s.handleGetAny(resp, req, &keyReq{
		key:   mux.Vars(req)[apiKey],
		cache: s.users.Get(mux.Vars(req)[apiKey]),
		get:   s.ui.GetInfo,
		save:  s.users.Save,
	})
}

func (s *server) handleGetAny(resp http.ResponseWriter, req *http.Request, keyReq *keyReq) {
	var (
		start = time.Now()
		when  = start
		user  *userinfo.UserInfo
		err   error
	)

	// Check if the cached data is nil or real.
	// If the cached data is nil, then pull fresh data.
	// If the fresh data pull has no error, then cache it.
	if keyReq.cache != nil && keyReq.cache.Data != nil {
		user, _ = keyReq.cache.Data.(*userinfo.UserInfo)
		when = keyReq.cache.Time
	} else if user, err = keyReq.get(req.Context(), keyReq.key); errors.Is(err, userinfo.ErrNoUser) {
		keyReq.save(keyReq.key, user, true)
	} else if err != nil {
		log.Printf("[ERROR] %v", err)
	} else {
		keyReq.save(keyReq.key, user, false)
	}

	if user == nil { // this only happens on error above.
		user = userinfo.DefaultUser()
		key, length := maskAPIKey(keyReq.key)
		log.Println("[ERROR] user missing from cache or lookup", key, length)
	}

	resp.Header().Set("X-API-Key", user.APIKey)
	resp.Header().Set("X-Environment", user.Environment)
	resp.Header().Set("X-Username", user.Username)
	resp.Header().Set("X-UserID", user.UserID)
	resp.Header().Set("Age", strconv.Itoa(int((time.Since(when).Seconds()))))
	resp.Header().Set("X-Request-Time", time.Since(start).Round(time.Millisecond).String())

	if user.UserID == userinfo.DefaultUserID && (err == nil || errors.Is(err, userinfo.ErrNoUser)) {
		s.noKeyReply(resp, req)
	} else {
		resp.WriteHeader(http.StatusOK)
	}
}

func (s *server) handleDelSrv(resp http.ResponseWriter, req *http.Request) {
	start := time.Now()
	user := userinfo.DefaultUser()
	item := s.servers.Get(req.Header.Get("X-Server"))

	defer s.servers.Delete(req.Header.Get("X-Server"))

	if item != nil && item.Data != nil {
		user, _ = item.Data.(*userinfo.UserInfo)
	}

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

	if err := json.NewEncoder(resp).Encode(item.Data); err != nil {
		log.Printf("[ERROR] writing response: %v", err)
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
		log.Printf("[ERROR] writing response: %v", err)
	}
}

func (s *server) handleUserStats(resp http.ResponseWriter, req *http.Request) {
	if item := s.users.Get(req.Header.Get("x-api-key")); item == nil || item.Data == nil {
		resp.WriteHeader(http.StatusNotFound)
	} else if err := json.NewEncoder(resp).Encode(item); err != nil {
		log.Printf("[ERROR] writing response: %v", err)
	}
}

func (s *server) handleSrvStats(resp http.ResponseWriter, req *http.Request) {
	if item := s.servers.Get(req.Header.Get("x-server")); item == nil || item.Data == nil {
		resp.WriteHeader(http.StatusNotFound)
	} else if err := json.NewEncoder(resp).Encode(item); err != nil {
		log.Printf("[ERROR] writing response: %v", err)
	}
}

// noKeyReply returns a 401.
func (s *server) noKeyReply(resp http.ResponseWriter, req *http.Request) {
	key, length := maskAPIKey(mux.Vars(req)[apiKey])
	resp.Header().Set("X-Key", key)
	resp.Header().Set("X-API-Key", mux.Vars(req)[apiKey])
	resp.Header().Set("X-Length", strconv.Itoa(length))
	resp.WriteHeader(http.StatusUnauthorized)
}
