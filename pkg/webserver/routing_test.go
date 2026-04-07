//nolint:testpackage // Tests unexported handleAuth.
package webserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleAuth_deleteWithoutHeadersIsNotFound(t *testing.T) {
	t.Parallel()

	s := &server{Config: &Config{}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/auth", nil)

	s.handleAuth(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestHandleAuth_unknownMethodIsNotFound(t *testing.T) {
	t.Parallel()

	s := &server{Config: &Config{}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPatch, "/auth", nil)

	s.handleAuth(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
