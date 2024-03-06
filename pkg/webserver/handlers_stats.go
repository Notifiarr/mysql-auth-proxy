package webserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Notifiarr/mysql-auth-proxy/docs"
	"github.com/gorilla/mux"
	"github.com/swaggo/swag"
)

/* This file contains the stats and other handlers. */

// @Description  Retrieve full cached user list.
// @Summary      Return all cached users
// @Tags         stats
// @Produce      json
// @Success      200  {object} map[string]cache.Item{data=userinfo.UserInfo} "List of cached API keys. The map key is the API key."
// @Failure      401  {object} string "invalid request"
// @Router       /stats/keys [get]
func (s *server) handeUserList(resp http.ResponseWriter, _ *http.Request) {
	if err := json.NewEncoder(resp).Encode(s.users.List()); err != nil {
		s.Printf("[ERROR] writing response: %v", err)
	}
}

// @Description  Retrieve a user's cached info.
// @Summary      Return cached user
// @Tags         stats
// @Produce      json
// @Param        key  path   string  true  "API Key"
// @Success      200  {object} cache.Item{data=userinfo.UserInfo} "User's cached info."
// @Failure      401  {object} string "invalid request"
// @Router       /stats/key/{key} [get]
func (s *server) handleUserInfo(resp http.ResponseWriter, req *http.Request) {
	if err := json.NewEncoder(resp).Encode(s.users.Get(mux.Vars(req)["key"])); err != nil {
		s.Printf("[ERROR] writing response: %v", err)
	}
}

// @Description  Re-reads the config file and updates the no-auth/no-api-key-required paths.
// @Summary      Updates no-api-required paths
// @Tags         config
// @Produce      json
// @Success      200  {object} string "config reloaded: true"
// @Failure      500  {object} string "error reading config"
// @Router       /reload [get]
func (s *server) reloadConfig(resp http.ResponseWriter, _ *http.Request) {
	config, err := LoadConfig(s.filePath)
	if err != nil {
		http.Error(resp, "Error loading config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.addCh <- config.NoAuthPaths
	http.Error(resp, fmt.Sprint("Config Reloaded: ", <-s.answer), http.StatusOK)
}

// @Description  Retrieve auth proxy configuration, minus passwords.
// @Summary      Return auth proxy config
// @Tags         config
// @Produce      json
// @Success      200  {object} Config "Auth Proxy Config"
// @Failure      401  {object} string "invalid request"
// @Router       /stats/config [get]
func (s *server) showConfig(resp http.ResponseWriter, _ *http.Request) {
	if err := json.NewEncoder(resp).Encode(s.Config); err != nil {
		s.Printf("[ERROR] writing response: %v", err)
	}
}

func (s *server) handlerSwaggerDoc(response http.ResponseWriter, request *http.Request) {
	instance := strings.TrimSuffix(mux.Vars(request)["instance"], ".json")
	if instance == "" {
		instance = "api"
	}

	docs.SwaggerInfoapi.Version = "v0"

	//	docs.SwaggerInfoapi.BasePath = c.Config.URLBase
	docs.SwaggerInfoapi.Host = request.Host

	doc, err := swag.ReadDoc(instance)
	if err != nil {
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}

	_, _ = response.Write([]byte(doc))
}
