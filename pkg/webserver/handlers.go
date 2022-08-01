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

type getReq struct {
	key  string
	item *cache.Item
	get  func(context.Context, string) (*userinfo.UserInfo, error)
	save func(string, interface{}) bool
}

func (s *server) handleServer(resp http.ResponseWriter, req *http.Request) {
	item, _ := s.servers.Get(req.Header.Get("X-Server"))
	s.handleGetAny(resp, req, &getReq{
		key:  req.Header.Get("X-Server"),
		item: item,
		get:  s.ui.GetServer,
		save: s.servers.Save,
	})
}

func (s *server) handleGetKey(resp http.ResponseWriter, req *http.Request) {
	item, _ := s.users.Get(mux.Vars(req)[apiKey])
	s.handleGetAny(resp, req, &getReq{
		key:  mux.Vars(req)[apiKey],
		item: item,
		get:  s.ui.GetInfo,
		save: s.users.Save,
	})
}

func (s *server) handleGetAny(resp http.ResponseWriter, req *http.Request, getReq *getReq) {
	var (
		start = time.Now()
		when  = start
		user  *userinfo.UserInfo
		err   error
	)

	if getReq.item != nil && getReq.item.Data != nil {
		user, _ = getReq.item.Data.(*userinfo.UserInfo)
		when = getReq.item.Time
	} else if user, err = getReq.get(req.Context(), getReq.key); err != nil && !errors.Is(err, userinfo.ErrNoUser) {
		log.Printf("[ERROR] %v", err)
	} else {
		getReq.save(getReq.key, user)
	}

	resp.Header().Set("X-API-Key", user.APIKey)
	resp.Header().Set("X-Environment", user.Environment)
	resp.Header().Set("X-Username", user.Username)
	resp.Header().Set("X-UserID", user.UserID)
	resp.Header().Set("Age", strconv.Itoa(int((time.Since(when).Seconds()))))
	resp.Header().Set("X-Request-Time", time.Since(start).Round(time.Millisecond).String())

	if user.UserID == userinfo.DefaultUserID && err == nil || errors.Is(err, userinfo.ErrNoUser) {
		s.noKeyReply(resp, req)
	} else {
		resp.WriteHeader(http.StatusOK)
	}
}

func (s *server) handleDelSrv(resp http.ResponseWriter, req *http.Request) {
	start := time.Now()
	user := userinfo.DefaultUser()

	item := s.servers.Delete(req.Header.Get("X-Server"))
	if item != nil && item.Data != nil {
		user, _ = item.Data.(*userinfo.UserInfo)
	}

	if user.UserID != userinfo.DefaultUserID {
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
		infos[idx] = s.users.Delete(key)
		if infos[idx] != nil && infos[idx].Data != nil {
			user, _ = infos[idx].Data.(*userinfo.UserInfo)
		}

		// Something is better than nothing.
		if user.UserID != userinfo.DefaultUserID {
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
