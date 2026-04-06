package webserver_test

import (
	"net/http"
	"testing"

	"github.com/Notifiarr/mysql-auth-proxy/pkg/webserver"
)

const testAPIKey = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

func TestSetRefererForOriginalURI(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, origURI, wantReferer string
	}{
		{
			name:        "nginx style path strips key segment",
			origURI:     "/api/v1/route/method/" + testAPIKey,
			wantReferer: "/api/v1/route/method",
		},
		{
			name:        "drops query",
			origURI:     "/api/v1/route/method/key?foo=bar&baz=1",
			wantReferer: "/api/v1/route/method",
		},
		{
			name:        "only query does not set referer",
			origURI:     "?foo=bar",
			wantReferer: "",
		},
		{
			name:        "empty does not set referer",
			origURI:     "",
			wantReferer: "",
		},
		{
			name:        "too few segments sets full path only",
			origURI:     "/api/v1/foo",
			wantReferer: "/api/v1/foo",
		},
		{
			name:        "double slash adds empty segment so keyPosition 5 is method not key",
			origURI:     "/api/v1//route/method/key",
			wantReferer: "/api/v1//route",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			h := http.Header{}
			h.Set("X-Original-Uri", testCase.origURI)
			webserver.SetRefererForOriginalURI(h)

			if got := h.Get("Referer"); got != testCase.wantReferer {
				t.Fatalf("Referer = %q, want %q (X-Original-Uri=%q)", got, testCase.wantReferer, testCase.origURI)
			}
		})
	}
}

func TestGetAPIKeyFromURIPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, pathStr, want string
	}{
		{
			name:    "key at segment keyPosition",
			pathStr: "/api/v1/route/method/" + testAPIKey,
			want:    testAPIKey,
		},
		{
			name:    "strips query from last segment when fused",
			pathStr: "/api/v1/route/method/" + testAPIKey + "?foo=bar",
			want:    testAPIKey,
		},
		{
			name:    "empty",
			pathStr: "",
			want:    "",
		},
		{
			name:    "too few segments",
			pathStr: "/api/v1/foo",
			want:    "",
		},
		{
			name:    "double slash shifts which segment is keyPosition",
			pathStr: "/api/v1//route/method/not-the-uuid-key",
			want:    "method",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := webserver.GetAPIKeyFromURIPath(testCase.pathStr); got != testCase.want {
				t.Fatalf("GetAPIKeyFromURIPath(%q) = %q, want %q", testCase.pathStr, got, testCase.want)
			}
		})
	}
}
