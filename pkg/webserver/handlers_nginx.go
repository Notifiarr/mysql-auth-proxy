package webserver

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Notifiarr/mysql-auth-proxy/pkg/userinfo"
	"github.com/gorilla/mux"
	"golift.io/cache"
)

/* The handlers in this file are used by Nginx. They only return headers. */

type keyReq struct {
	key   string
	cache *cache.Item
	get   func(context.Context, string) (*userinfo.UserInfo, error)
	save  func(string, interface{}, cache.Options) bool
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
		keyReq.save(keyReq.key, user, cache.Options{Prune: true})
	} else if err != nil {
		s.Printf("[ERROR] %v", err)
	} else {
		keyReq.save(keyReq.key, user, cache.Options{Prune: false})
	}

	if user == nil { // this only happens on error above.
		user = userinfo.DefaultUser()
		key, length := maskAPIKey(keyReq.key)
		s.Println("[ERROR] user missing from cache or lookup", key, length)
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
