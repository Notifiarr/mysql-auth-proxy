package webserver

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/Notifiarr/mysql-auth-proxy/pkg/userinfo"
	"golift.io/cache"
)

/* This file contains the delete handlers. Used by automation. */

type noExists struct {
	Exists bool `json:"exists"`
}

// @Description  Delete API Keys or Server IDs from internal cache.  Sending X-Server header deletes a server form cache, and sending X-Api-Keys header deletes API keys from cache.
// @Summary      Delete Cache Entries
// @Tags         auth
// @Produce      json
// @Param        X-Server   header string true "Discord Server ID to delete."
// @Param        X-Api-Keys header string true "Comma separated list of API keys to delete."
// @Success      200  {object} []cache.Item{data=userinfo.UserInfo} "List of cached info for API Keys or servers that were deleted."
// @Success      208  {object} noExists "exists: false is returned when a missing server ID is provided."
// @Failure      401  {object} string "invalid request"
// @Router       /auth [delete]
func (s *server) handleDelSrv(resp http.ResponseWriter, req *http.Request) {
	user := userinfo.DefaultUser()

	serverID := getHeader(req.Header, HeaderXServer)

	item := s.servers.Get(serverID)
	if item != nil && item.Data != nil {
		if u, ok := item.Data.(*userinfo.UserInfo); ok {
			user = u
		}
	}

	defer s.servers.Delete(serverID)

	// These headers are mostly for logs.
	if user != nil && user.UserID != userinfo.DefaultUserID {
		resp.Header().Set(HeaderXUserid, user.UserID)
		resp.Header().Set(HeaderXUsername, user.Username)
	}

	resp.Header().Set(HeaderEnvironment, "deleted")
	resp.Header().Set(HeaderContentType, "application/json")
	resp.Header().Set(HeaderAge, "1")

	var reply any = noExists{}
	if item != nil {
		reply = []*cache.Item{item}
		resp.WriteHeader(http.StatusOK)
	} else {
		resp.WriteHeader(http.StatusAlreadyReported)
	}

	err := json.NewEncoder(resp).Encode(reply)
	if err != nil {
		s.Printf("[ERROR] writing response: %v", err)
	}
}

func (s *server) handleDelKey(resp http.ResponseWriter, req *http.Request) {
	keys := strings.Split(getHeader(req.Header, HeaderXAPIKeys), ",")
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
			resp.Header().Set(HeaderXUserid, user.UserID)
			resp.Header().Set(HeaderXUsername, user.Username)
		}
	}

	resp.Header().Set(HeaderEnvironment, "deleted")
	resp.Header().Set(HeaderContentType, "application/json")
	resp.Header().Set(HeaderAge, strconv.Itoa(len(infos)))
	resp.WriteHeader(http.StatusOK)

	err := json.NewEncoder(resp).Encode(infos)
	if err != nil {
		s.Printf("[ERROR] writing response: %v", err)
	}
}
