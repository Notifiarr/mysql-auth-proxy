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

// alexaCacheFor is how long a successful token lookup may be reused.
// It is also a ceiling: the entry is dropped at access_expires when that is sooner.
// The short window is what makes an unlink or a rotated token fail closed
// without waiting for the original expiry.
const alexaCacheFor = time.Minute

// alexaCached is a successful lookup plus the time it must be rechecked.
type alexaCached struct {
	user    *userinfo.UserInfo
	expires time.Time
}

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

	if user, when, hit := s.cachedAlexa(token); hit {
		s.writeAuthResult(resp, req, "alexa", user, nil, when, start)
		return
	}

	user, expires, err := s.ui.GetAlexa(req.Context(), token)
	s.cacheAlexa(token, user, expires, err)

	if user == nil {
		user = userinfo.DefaultUser()
		s.Println("[ERROR] alexa user missing from cache or lookup")
	}

	s.writeAuthResult(resp, req, "alexa", user, err, start, start)
}

// cachedAlexa returns a lookup that is still inside its recheck window.
func (s *server) cachedAlexa(token string) (*userinfo.UserInfo, time.Time, bool) {
	if s.alexa == nil {
		return nil, time.Time{}, false
	}

	var snap cache.Item
	if !s.alexa.GetInto(token, &snap) || snap.Data == nil {
		return nil, time.Time{}, false
	}

	entry, ok := snap.Data.(*alexaCached)
	if !ok || entry == nil || entry.user == nil || !time.Now().Before(entry.expires) {
		return nil, time.Time{}, false
	}

	return entry.user, snap.Time, true
}

func (s *server) cacheAlexa(token string, user *userinfo.UserInfo, expires time.Time, err error) {
	if err != nil || user == nil {
		if err != nil && !errors.Is(err, userinfo.ErrNoUser) {
			s.Printf("[ERROR] %v", err)
		}

		return
	}

	until := alexaCacheDeadline(expires, time.Now())
	s.alexa.Save(token, &alexaCached{user: user, expires: until}, cache.Options{Expire: until})
}

// alexaCacheDeadline is the sooner of the token expiry and now plus alexaCacheFor.
func alexaCacheDeadline(accessExpires, now time.Time) time.Time {
	deadline := now.Add(alexaCacheFor)
	if !accessExpires.IsZero() && accessExpires.Before(deadline) {
		return accessExpires
	}

	return deadline
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
