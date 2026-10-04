//nolint:testpackage // Tests unexported alexaAccessToken and handleAlexa.
package webserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Notifiarr/mysql-auth-proxy/pkg/userinfo"
	"golift.io/cache"
)

func TestAlexaAccessToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "session",
			body: `{"session":{"user":{"accessToken":"from-session"}}}`,
			want: "from-session",
		},
		{
			name: "context fallback",
			body: `{"context":{"System":{"user":{"accessToken":"from-context"}}}}`,
			want: "from-context",
		},
		{
			name: "session wins",
			body: `{"session":{"user":{"accessToken":"from-session"}},"context":{"System":{"user":{"accessToken":"from-context"}}}}`,
			want: "from-session",
		},
		{
			name: "missing",
			body: `{"request":{"type":"LaunchRequest"}}`,
			want: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := alexaAccessToken(strings.NewReader(test.body))
			if err != nil {
				t.Fatalf("alexaAccessToken() error = %v", err)
			}

			if got != test.want {
				t.Fatalf("alexaAccessToken() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestAlexaAccessToken_invalidJSON(t *testing.T) {
	t.Parallel()

	_, err := alexaAccessToken(strings.NewReader("{"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestHandleAlexa_missingTokenIsUnauthorized(t *testing.T) {
	t.Parallel()

	s := &server{Config: &Config{}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/auth/alexa", strings.NewReader(`{"version":"1.0"}`))

	s.handleAlexa(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}

	if rec.Header().Get(HeaderXUserid) != "-1" {
		t.Fatalf("X-Userid = %q, want -1", rec.Header().Get(HeaderXUserid))
	}
}

func TestAuthAlexaRouteDoesNotHitAPIKeyAuth(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/auth", func(resp http.ResponseWriter, _ *http.Request) {
		resp.WriteHeader(http.StatusTeapot)
	})
	mux.HandleFunc("/auth/alexa", func(resp http.ResponseWriter, _ *http.Request) {
		resp.WriteHeader(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/auth/alexa", strings.NewReader(`{}`))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func TestAlexaCacheDeadline(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0)
	soon := now.Add(10 * time.Second)
	later := now.Add(2 * time.Hour)

	if got := alexaCacheDeadline(soon, now); !got.Equal(soon) {
		t.Fatalf("deadline = %s, want token expiry %s", got, soon)
	}

	if got := alexaCacheDeadline(later, now); !got.Equal(now.Add(alexaCacheFor)) {
		t.Fatalf("deadline = %s, want %s", got, now.Add(alexaCacheFor))
	}
}

func TestCachedAlexa_pastDeadlineIsMiss(t *testing.T) {
	t.Parallel()

	store := cache.New(cache.Config{PruneInterval: time.Hour})
	t.Cleanup(func() { store.Stop(false) })

	past := time.Now().Add(-time.Second)
	store.Save("tok", &alexaCached{user: userinfo.DefaultUser(), expires: past}, cache.Options{Expire: past})

	s := &server{alexa: store}
	if _, _, hit := s.cachedAlexa("tok"); hit {
		t.Fatal("expected expired cache entry to miss")
	}
}

func TestHandleAlexa_unknownMethodIsNotFound(t *testing.T) {
	t.Parallel()

	s := &server{Config: &Config{}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/auth/alexa", nil)

	s.handleAlexa(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
