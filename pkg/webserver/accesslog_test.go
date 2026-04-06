//nolint:testpackage // Tests require access to responseWriter and the access log line writers.
package webserver

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

const accessLogTestAPIKey = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

// writeLogLine mirrors responseWriter.writeAccessLogLine (prefix + tail) for unit tests.
func writeLogLine(builder *strings.Builder, req *http.Request, writer *responseWriter) {
	builder.Grow(accessLogInitialGrow)
	writer.writeAccessLogLinePrefix(builder, req)
	writer.writeAccessLogLineTail(builder, req)
}

func TestResponseWriter(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	capWriter := &responseWriter{ResponseWriter: rec}

	if capWriter.statusCode() != http.StatusOK {
		t.Fatalf("statusCode before any write = %d, want 200", capWriter.statusCode())
	}

	capWriter.WriteHeader(http.StatusTeapot)

	if capWriter.status != http.StatusTeapot {
		t.Fatalf("status after WriteHeader = %d", capWriter.status)
	}

	capWriter.WriteHeader(http.StatusBadRequest) // second WriteHeader ignored

	if capWriter.status != http.StatusTeapot {
		t.Fatalf("second WriteHeader should be ignored, got status %d", capWriter.status)
	}

	written, err := capWriter.Write([]byte("ab"))
	if err != nil {
		t.Fatal(err)
	}

	if written != 2 {
		t.Fatalf("Write n = %d", written)
	}

	if capWriter.size != 2 {
		t.Fatalf("size = %d, want 2", capWriter.size)
	}

	if capWriter.statusCode() != http.StatusTeapot {
		t.Fatalf("statusCode = %d, want %d", capWriter.statusCode(), http.StatusTeapot)
	}
}

func TestResponseWriter_WriteImpliesOK(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	capWriter := &responseWriter{ResponseWriter: rec}

	_, err := capWriter.Write([]byte("x"))
	if err != nil {
		t.Fatal(err)
	}

	if capWriter.status != http.StatusOK {
		t.Fatalf("Write without WriteHeader: status = %d, want 200", capWriter.status)
	}

	if capWriter.statusCode() != http.StatusOK {
		t.Fatalf("statusCode = %d, want 200", capWriter.statusCode())
	}
}

func TestWriteAccessLogLine(t *testing.T) {
	t.Parallel()

	elapsed := 42*time.Millisecond + 500*time.Microsecond // Milliseconds() truncates to 42

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/p?q=1", nil)
	req.RequestURI = "/p?q=1"
	req.RemoteAddr = "192.0.2.1:9999"
	req.Header.Set("User-Agent", "test-agent/1")
	req.Header.Set("X-Server", "srv-99")
	req.Header.Set("X-Original-Uri", "/api/v1/route/method/"+accessLogTestAPIKey)

	rec := httptest.NewRecorder()
	capWriter := &responseWriter{ResponseWriter: rec}
	capWriter.start = time.Now().Add(-elapsed)
	capWriter.Header().Set("X-Username", "alice")
	capWriter.Header().Set("X-Userid", "1001")
	capWriter.Header().Set("Age", "60")
	capWriter.Header().Set("X-Environment", "live")
	capWriter.WriteHeader(http.StatusOK)
	_, _ = capWriter.Write([]byte("body"))

	builder := &strings.Builder{}

	writeLogLine(builder, req, capWriter)

	got := builder.String()
	ts := capWriter.start.Format("02/Jan/2006:15:04:05 -0700")
	wantPrefix := fmt.Sprintf(
		`example.com 192.0.2.1 "alice" 1001 [%s] "GET /p?q=1 HTTP/1.1" 200 4 "/api/v1/route/method" "test-agent/1"`,
		ts,
	)

	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("line prefix mismatch\ngot:  %q\nwant: %q", got, wantPrefix)
	}

	rest := got[len(wantPrefix):]
	reqMs := regexp.MustCompile(`^ req:([0-9]+)ms `).FindStringSubmatch(rest)

	if len(reqMs) != 2 {
		t.Fatalf("expected req:NNms after prefix, got rest: %q", rest)
	}

	elapsedMs, err := strconv.Atoi(reqMs[1])
	if err != nil {
		t.Fatal(err)
	}

	if elapsedMs < 40 || elapsedMs > 45 {
		t.Fatalf("req duration ms = %d, want ~42 (40–45)", elapsedMs)
	}

	wantTail := `age:60 env:live key:() "srv:srv-99"` + "\n"
	if !strings.HasSuffix(got, wantTail) {
		t.Fatalf("line tail mismatch\ngot:  %q\nwant suffix: %q", got, wantTail)
	}
}

func TestWriteAccessLogLine_RequestURI(t *testing.T) {
	t.Parallel()

	start := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://ignored/", nil)
	req.RequestURI = "/raw%20path"
	req.RemoteAddr = "127.0.0.1:1"

	rec := httptest.NewRecorder()
	capWriter := &responseWriter{ResponseWriter: rec}
	capWriter.start = start

	builder := &strings.Builder{}

	writeLogLine(builder, req, capWriter)

	got := builder.String()
	if !strings.Contains(got, `"GET /raw%20path HTTP/1.1"`) {
		t.Fatalf("expected RequestURI in log line, got: %q", got)
	}
}

func TestWriteAccessLogLine_MaskedKeyFromResponseHeader(t *testing.T) {
	t.Parallel()

	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "http://x/", nil)
	req.Header.Set("X-Server", "")

	rec := httptest.NewRecorder()
	capWriter := &responseWriter{ResponseWriter: rec}
	capWriter.start = start
	capWriter.Header().Set("X-Api-Key", accessLogTestAPIKey)

	builder := &strings.Builder{}

	writeLogLine(builder, req, capWriter)

	masked, keyLenStr := maskAPIKey(accessLogTestAPIKey)
	want := "key:" + masked + "(" + keyLenStr + ")"

	if !strings.Contains(builder.String(), want) {
		t.Fatalf("expected masked key from X-Api-Key response header in line: %q", builder.String())
	}
}

func TestWriteAccessLogLine_NoAPIKeyResponseHeader(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://h/", nil)

	rec := httptest.NewRecorder()
	capWriter := &responseWriter{ResponseWriter: rec}
	capWriter.start = time.Now().Add(-5 * time.Millisecond)

	builder := &strings.Builder{}

	writeLogLine(builder, req, capWriter)

	if !strings.Contains(builder.String(), `key:()`) {
		t.Fatalf("expected empty key segment, got: %q", builder.String())
	}
}

func TestAccessLogWrap(t *testing.T) {
	t.Parallel()

	var dst bytes.Buffer

	handler := accessLogWrap(http.HandlerFunc(func(resp http.ResponseWriter, _ *http.Request) {
		resp.Header().Set("X-Username", "wrap-user")
		resp.Header().Set("X-Userid", "55")
		resp.Header().Set("Age", "3")
		resp.Header().Set("X-Environment", "dev")
		resp.WriteHeader(http.StatusNoContent)
	}), &dst)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://proxy.test/auth", nil)
	req.RequestURI = "/auth"
	req.RemoteAddr = "198.51.100.2:4444"
	req.Header.Set("X-Forwarded-For", "203.0.113.5")
	req.Header.Set("X-Server", "discord-srv")
	req.Header.Set("User-Agent", "ua-wrap")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	line := dst.String()
	if !strings.HasPrefix(line, `proxy.test 203.0.113.5 "wrap-user" 55 [`) {
		t.Fatalf("unexpected line start: %q", line)
	}

	if !strings.Contains(line, `"GET /auth HTTP/1.1" 204 0`) {
		t.Fatalf("expected request line and 204 in log: %q", line)
	}

	if !strings.Contains(line, `"ua-wrap"`) {
		t.Fatalf("expected quoted User-Agent in log: %q", line)
	}

	if !strings.Contains(line, "ms age:") {
		t.Fatalf("expected req:…ms age: in log: %q", line)
	}

	if !strings.Contains(line, `age:3 env:dev key:() "srv:discord-srv"`) {
		t.Fatalf("expected tail fields: %q", line)
	}
}

func TestAccessLogWrap_MaskedKeyFromResponseHeader(t *testing.T) {
	t.Parallel()

	var dst bytes.Buffer

	handler := accessLogWrap(http.HandlerFunc(func(resp http.ResponseWriter, _ *http.Request) {
		resp.Header().Set("X-Api-Key", accessLogTestAPIKey)
		resp.WriteHeader(http.StatusUnauthorized)
	}), &dst)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://h/", nil)
	req.RemoteAddr = "127.0.0.1:1"

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	masked, keyLenStr := maskAPIKey(accessLogTestAPIKey)
	want := "key:" + masked + "(" + keyLenStr + ")"

	if !strings.Contains(dst.String(), want) {
		t.Fatalf("expected masked key from X-Api-Key response header in log: %q", dst.String())
	}
}
