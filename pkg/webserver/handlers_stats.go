package webserver

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
)

/* This file contains the stats and other handlers. */

func (s *server) handeUserList(resp http.ResponseWriter, req *http.Request) {
	if err := json.NewEncoder(resp).Encode(s.users.List()); err != nil {
		s.Printf("[ERROR] writing response: %v", err)
	}
}

func (s *server) handleUserInfo(resp http.ResponseWriter, req *http.Request) {
	if err := json.NewEncoder(resp).Encode(s.users.Get(mux.Vars(req)["key"])); err != nil {
		s.Printf("[ERROR] writing response: %v", err)
	}
}

func (s *server) handeSrvList(resp http.ResponseWriter, req *http.Request) {
	if err := json.NewEncoder(resp).Encode(s.servers.List()); err != nil {
		s.Printf("[ERROR] writing response: %v", err)
	}
}

func (s *server) handleSrvInfo(resp http.ResponseWriter, req *http.Request) {
	if err := json.NewEncoder(resp).Encode(s.servers.Get(mux.Vars(req)["key"])); err != nil {
		s.Printf("[ERROR] writing response: %v", err)
	}
}

// noKeyReply returns a 401.
func (s *server) noKeyReply(resp http.ResponseWriter, req *http.Request) {
	key, length := maskAPIKey(mux.Vars(req)[apiKey])
	resp.Header().Set("X-Key", key)
	resp.Header().Set("X-API-Key", mux.Vars(req)[apiKey])
	resp.Header().Set("X-Length", strconv.Itoa(length))

	if s.RequiresAPIKey(req.URL.Path) {
		resp.WriteHeader(http.StatusUnauthorized)
	} else {
		resp.WriteHeader(http.StatusOK)
	}
}

func (s *server) reloadConfig(resp http.ResponseWriter, req *http.Request) {
	config, err := LoadConfig(s.filePath)
	if err != nil {
		http.Error(resp, "Error loading config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	for _, uriPath := range config.NoAuthPaths {
		s.addCh <- uriPath
		<-s.answer
	}

	http.Error(resp, "Config Reloaded", http.StatusOK)
}

func (s *server) showConfig(resp http.ResponseWriter, req *http.Request) {
	if err := json.NewEncoder(resp).Encode(s.Config); err != nil {
		s.Printf("[ERROR] writing response: %v", err)
	}
}
