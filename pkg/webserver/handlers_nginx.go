package webserver

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Notifiarr/mysql-auth-proxy/pkg/userinfo"
	"golift.io/cache"
)

/* The handlers in this file are used by Nginx. They only return headers. */

type keyReq struct {
	label string
	key   string
	store *cache.Cache
	get   func(context.Context, string) (*userinfo.UserInfo, error)
	save  func(string, any, cache.Options) bool
}

// cacheUserFromGetInto loads a cached *userinfo.UserInfo and its save time without allocating *cache.Item.
func cacheUserFromGetInto(store *cache.Cache, key string) (*userinfo.UserInfo, time.Time, bool) {
	var snap cache.Item
	if !store.GetInto(key, &snap) || snap.Data == nil {
		return nil, time.Time{}, false
	}

	user, ok := snap.Data.(*userinfo.UserInfo)
	if !ok || user == nil {
		return nil, time.Time{}, false
	}

	return user, snap.Time, true
}

func (s *server) handleServer(resp http.ResponseWriter, req *http.Request) {
	key := getHeader(req.Header, "X-Server")
	s.handleGetAny(resp, req, keyReq{
		label: "servers",
		key:   key,
		store: s.servers,
		get:   s.ui.GetServer,
		save:  s.servers.Save,
	})
}

func (s *server) handleGetKey(resp http.ResponseWriter, req *http.Request) {
	key := apiKeyFromRequest(req)
	s.handleGetAny(resp, req, keyReq{
		label: "users",
		key:   key,
		store: s.users,
		get:   s.ui.GetInfo,
		save:  s.users.Save,
	})
}

// @Description  Retrieve the environment for an API Key or Server ID. This endpoint is designed for auth proxy requests from Nginx.
// @Description One of X-Server, X-Api-Key or X-Original-URI (with an api key in it) must be provided.
// @Summary      Get user or server environment
// @Tags         auth
// @Param        X-Server       header string false "Discord Server ID to route."
// @Param        X-Password     header string false "Shared website secret. Required when X-Server header is provided."
// @Param        X-Api-Key      header string false "User's API Key to route. May also be provided in X-Original-URI header."
// @Param        X-Original-URI header string false "User's API Key may be provided in this header at URI position 5: /api/v1/route/method/{key}"
// @Success      200                         "Body is empty on success, check headers."
// @Header       200 {string} X-Api-Key      "API Key parsed from request."
// @Header       200 {string} X-Environment  "Environment: live, dev, etc."
// @Header       200 {string} X-Username     "Username for the user whose API key was provided."
// @Header       200 {string} X-UserID       "MySQL ID for the user whose API key was provided."
// @Header       200 {string} Age            "How long this information has been in the cache."
// @Failure      401 {object} string         "invalid request"
// @Header       401 {string} X-Api-Key      "API Key parsed from request."
// @Router       /auth [get]
func (s *server) handleGetAny(resp http.ResponseWriter, req *http.Request, keyReq keyReq) {
	var (
		start           = time.Now()
		err             error
		user, when, hit = cacheUserFromGetInto(keyReq.store, keyReq.key)
	)

	if !hit {
		when = start

		switch user, err = keyReq.get(req.Context(), keyReq.key); {
		case errors.Is(err, userinfo.ErrNoUser):
			keyReq.save(keyReq.key, user, cache.Options{Prune: true}) // save the "default user" to the cache.
		case err != nil:
			s.Printf("[ERROR] %v", err) // database error.
		default:
			keyReq.save(keyReq.key, user, cache.Options{Prune: false}) // save the valid user to the cache.
		}

		if user == nil { // this only happens on error above.
			user = userinfo.DefaultUser()
			key, length := maskAPIKey(keyReq.key)
			s.Println("[ERROR] user missing from cache or lookup", key, length)
		}
	}

	s.writeAuthResult(resp, req, keyReq.label, user, err, when, start)
}

func (s *server) writeAuthResult(
	resp http.ResponseWriter,
	req *http.Request,
	label string,
	user *userinfo.UserInfo,
	err error,
	when time.Time,
	start time.Time,
) {
	finished := time.Now()
	s.metrics.ReqTime.WithLabelValues(label).Observe(finished.Sub(start).Seconds())
	resp.Header().Set(HeaderXAPIKey, user.APIKey)
	resp.Header().Set(HeaderEnvironment, user.Environment)
	resp.Header().Set(HeaderXUsername, user.Username)
	resp.Header().Set(HeaderXUserid, user.UserID)
	resp.Header().Set(HeaderAge, strconv.Itoa(int(finished.Sub(when).Seconds())))
	// If the user is the default user, and there was no error, then return a 401.
	if user.UserID == userinfo.DefaultUserID && (err == nil || errors.Is(err, userinfo.ErrNoUser)) {
		s.noKeyReply(resp, req)
	} else {
		// This may not be right: Server misses may return 200, confirm?
		resp.WriteHeader(http.StatusOK)
	}
}

// noKeyReply returns a 401.
func (s *server) noKeyReply(resp http.ResponseWriter, req *http.Request) {
	resp.Header().Set(HeaderXAPIKey, apiKeyFromRequest(req))

	if s.RequiresAPIKey(getHeader(req.Header, HeaderXOriginalURI)) {
		resp.WriteHeader(http.StatusUnauthorized)
	} else {
		resp.WriteHeader(http.StatusOK)
	}
}

// handleAuth dispatches /auth by method and headers. This is our primary entry point.
func (s *server) handleAuth(resp http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodDelete:
		if getHeader(req.Header, HeaderXAPIKeys) != "" {
			s.handleDelKey(resp, req)
			return
		}

		if getHeader(req.Header, HeaderXServer) != "" {
			s.handleDelSrv(resp, req)
			return
		}

		http.NotFound(resp, req)
	case http.MethodGet, http.MethodHead:
		if getHeader(req.Header, HeaderXServer) != "" && getHeader(req.Header, HeaderXAPIKey) == s.Password {
			s.handleServer(resp, req)
			return
		}

		s.parseAPIKey(http.HandlerFunc(s.handleGetKey)).ServeHTTP(resp, req)
	case http.MethodPost, http.MethodPut:
		s.parseAPIKey(http.HandlerFunc(s.handleGetKey)).ServeHTTP(resp, req)
	default:
		http.NotFound(resp, req)
	}
}
