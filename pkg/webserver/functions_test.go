package webserver_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/Notifiarr/mysql-auth-proxy/pkg/webserver"
)

const testAPIKey = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

func TestRefererPathForLog(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, origURI, want string
	}{
		{
			name:    "nginx style path strips key segment",
			origURI: "/api/v1/route/method/" + testAPIKey,
			want:    "/api/v1/route/method",
		},
		{
			name:    "drops query",
			origURI: "/api/v1/route/method/key?foo=bar&baz=1",
			want:    "/api/v1/route/method",
		},
		{
			name:    "only query",
			origURI: "?foo=bar",
			want:    "",
		},
		{
			name:    "empty",
			origURI: "",
			want:    "",
		},
		{
			name:    "too few segments returns path only",
			origURI: "/api/v1/foo",
			want:    "/api/v1/foo",
		},
		{
			name:    "double slash adds empty segment so keyPosition 5 is method not key",
			origURI: "/api/v1//route/method/key",
			want:    "/api/v1//route",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			h := http.Header{}
			h.Set("X-Original-Uri", testCase.origURI)

			if got := webserver.RefererPathForLog(h); got != testCase.want {
				t.Fatalf("RefererPathForLog = %q, want %q (X-Original-Uri=%q)", got, testCase.want, testCase.origURI)
			}
		})
	}
}

func TestClientIPForLog(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/", nil)
	if err != nil {
		t.Fatal(err)
	}

	req.RemoteAddr = "192.0.2.1:12345"

	if got := webserver.ClientIPForLog(req); got != "192.0.2.1" {
		t.Fatalf("ClientIPForLog = %q, want 192.0.2.1", got)
	}

	req.Header.Set("X-Forwarded-For", " 203.0.113.9 , 198.51.100.1 ")

	if got := webserver.ClientIPForLog(req); got != "203.0.113.9" {
		t.Fatalf("ClientIPForLog with XFF = %q, want 203.0.113.9", got)
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
