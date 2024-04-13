package webserver

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Notifiarr/mysql-auth-proxy/pkg/userinfo"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"golift.io/cache"
)

/* The handlers in this file are used by Nginx. They only return headers. */

type keyReq struct {
	label string
	key   string
	cache *cache.Item
	get   func(context.Context, string) (*userinfo.UserInfo, error)
	save  func(string, interface{}, cache.Options) bool
}

func (s *server) handleServer(resp http.ResponseWriter, req *http.Request) {
	s.handleGetAny(resp, req, &keyReq{
		label: "servers",
		key:   req.Header.Get("X-Server"),
		cache: s.servers.Get(req.Header.Get("X-Server")),
		get:   s.ui.GetServer,
		save:  s.servers.Save,
	})
}

func (s *server) handleGetKey(resp http.ResponseWriter, req *http.Request) {
	s.handleGetAny(resp, req, &keyReq{
		label: "users",
		key:   mux.Vars(req)[apiKey],
		cache: s.users.Get(mux.Vars(req)[apiKey]),
		get:   s.ui.GetInfo,
		save:  s.users.Save,
	})
}

// @Description  Retrieve the environment for an API Key or Server ID. This endpoint is designed for auth proxy requests from Nginx.
// @Description One of X-Server, X-API-Key or X-Original-URI (with an api key in it) must be provided.
// @Summary      Get user or server environment
// @Tags         auth
// @Param        X-Server       header string false "Discord Server ID to route."
// @Param        X-Password     header string false "Shared website secret. Required when X-Server header is provided."
// @Param        X-API-Key      header string false "User's API Key to route. May also be provided in X-Original-URI header."
// @Param        X-Original-URI header string false "User's API Key may be provided in this header at URI position 5: /api/v1/route/method/{key}"
// @Success      200                         "Body is empty on success, check headers."
// @Header       200 {string} X-API-Key      "API Key parsed from request."
// @Header       200 {string} X-Environment  "Environment: live, dev, etc."
// @Header       200 {string} X-Username     "Username for the user whose API key was provided."
// @Header       200 {string} X-UserID       "MySQL ID for the user whose API key was provided."
// @Header       200 {string} Age            "How long this information has been in the cache."
// @Header       200 {string} X-Request-Time "How long the request elapsed."
// @Failure      401 {object} string         "invalid request"
// @Header       401 {string} X-Key          "Masked API Key parsed from request."
// @Header       401 {string} X-API-Key      "API Key parsed from request."
// @Header       401 {int}    X-Length       "The length of the API key."
// @Router       /auth [get]
func (s *server) handleGetAny(resp http.ResponseWriter, req *http.Request, keyReq *keyReq) {
	var (
		start = prometheus.NewTimer(s.metrics.ReqTime.WithLabelValues(keyReq.label))
		when  = time.Now()
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
	resp.Header().Set("X-Request-Time", start.ObserveDuration().Round(time.Millisecond).String())

	if user.UserID == userinfo.DefaultUserID && (err == nil || errors.Is(err, userinfo.ErrNoUser)) {
		s.noKeyReply(resp, req)
	} else {
		resp.WriteHeader(http.StatusOK)
	}
}

// noKeyReply returns a 401.
func (s *server) noKeyReply(resp http.ResponseWriter, req *http.Request) {
	key, length := maskAPIKey(mux.Vars(req)[apiKey])
	resp.Header().Set("X-Key", key)
	resp.Header().Set("X-API-Key", mux.Vars(req)[apiKey])
	resp.Header().Set("X-Length", strconv.Itoa(length))

	if s.RequiresAPIKey(req.Header.Get("X-Original-URI")) {
		resp.WriteHeader(http.StatusUnauthorized)
	} else {
		resp.WriteHeader(http.StatusOK)
	}
}
