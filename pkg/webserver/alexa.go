package webserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Notifiarr/mysql-auth-proxy/pkg/exp"
	"github.com/Notifiarr/mysql-auth-proxy/pkg/userinfo"
	"golift.io/cache"
)

// Alexa skill requests are a few kilobytes. Cap the body so a client cannot
// force the proxy to buffer an unbounded payload.
const alexaBodyLimit = 1 << 20

// alexaSkillRequest is the slice of an Alexa request that carries the account link.
// Amazon sends accessToken on session.user, and copies it to context.System.user.
type alexaSkillRequest struct {
	Session struct {
		User struct {
			AccessToken string `json:"accessToken"`
		} `json:"user"`
	} `json:"session"`
	Context struct {
		System struct {
			User struct {
				AccessToken string `json:"accessToken"`
			} `json:"user"`
		} `json:"System"` //nolint:tagliatelle // Alexa sends this object as System.
	} `json:"context"`
}

// handleAlexa authorizes a skill request from its JSON body.
// The body is the Alexa request. The link token is session.user.accessToken.
//
// @Description  Authorize an Alexa skill request. The body is the Alexa JSON request. The link token is read from session.user.accessToken, then context.System.user.accessToken. A current alexa_oauth row returns the same headers as /auth.
// @Summary      Authorize an Alexa access token
// @Tags         auth
// @Accept       json
// @Success      200 "Body is empty on success, check headers."
// @Header       200 {string} X-Api-Key     "Account API key for the linked user."
// @Header       200 {string} X-Environment "Environment: live, dev, nightly, etc."
// @Header       200 {string} X-Username    "Username for the linked user."
// @Header       200 {string} X-UserID      "MySQL ID for the linked user."
// @Header       200 {string} Age           "How long this information has been in the cache."
// @Failure      401 "missing, unknown, or expired Alexa access token"
// @Router       /auth/alexa [post]
func (s *server) handleAlexa(resp http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut:
	default:
		http.NotFound(resp, req)
		return
	}

	token, err := alexaAccessToken(req.Body)
	if err != nil || token == "" {
		if s.metrics != nil {
			s.metrics.HTTPRequests.WithLabelValues(exp.HTTPEventInvalidKey).Inc()
		}

		s.alexaDenied(resp)

		return
	}

	s.lookupAlexa(resp, req, token)
}

func (s *server) alexaDenied(resp http.ResponseWriter) {
	resp.Header().Set(HeaderXAPIKey, "")
	resp.Header().Set(HeaderEnvironment, userinfo.DefaultEnvironment)
	resp.Header().Set(HeaderXUsername, userinfo.DefaultUsername)
	resp.Header().Set(HeaderXUserid, userinfo.DefaultUserID)
	resp.WriteHeader(http.StatusUnauthorized)
}

func (s *server) lookupAlexa(resp http.ResponseWriter, req *http.Request, token string) {
	start := time.Now()
	user, when, hit := cacheUserFromGetInto(s.alexa, token)

	var err error

	if !hit {
		when = start
		user, err = s.ui.GetAlexa(req.Context(), token)
		s.cacheAlexa(token, user, err)

		if user == nil {
			user = userinfo.DefaultUser()
			s.Println("[ERROR] alexa user missing from cache or lookup")
		}
	}

	s.writeAuthResult(resp, req, "alexa", user, err, when, start)
}

func (s *server) cacheAlexa(token string, user *userinfo.UserInfo, err error) {
	switch {
	case errors.Is(err, userinfo.ErrNoUser):
		s.alexa.Save(token, user, cache.Options{Prune: true})
	case err != nil:
		s.Printf("[ERROR] %v", err)
	default:
		s.alexa.Save(token, user, cache.Options{Prune: false})
	}
}

// alexaAccessToken reads session.user.accessToken, then context.System.user.accessToken.
func alexaAccessToken(body io.Reader) (string, error) {
	if body == nil {
		return "", io.EOF
	}

	var skill alexaSkillRequest

	err := json.NewDecoder(io.LimitReader(body, alexaBodyLimit)).Decode(&skill)
	if err != nil {
		return "", fmt.Errorf("decoding alexa request: %w", err)
	}

	if skill.Session.User.AccessToken != "" {
		return skill.Session.User.AccessToken, nil
	}

	return skill.Context.System.User.AccessToken, nil
}
