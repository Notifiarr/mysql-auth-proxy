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

// @Description  Delete API Keys from internal cache.
// @Summary      Delete Cache Entries
// @Tags         auth
// @Produce      json
// @Param        X-API-Keys header string true "Comma separated list of API keys to delete."
// @Success      200  {object} []cache.Item{data=userinfo.UserInfo} "List of cached info for API Keys that were deleted."
// @Failure      401  {object} string "invalid request"
// @Router       /auth [delete]
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
