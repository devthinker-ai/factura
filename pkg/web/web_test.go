package web_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/devthinker-ai/factura/pkg/web"
)

func TestHandlerFromFS_Placeholder(t *testing.T) {
	h := web.HandlerFromFS(fstest.MapFS{
		"placeholder.txt": &fstest.MapFile{Data: []byte("x")},
	})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "make frontend") {
		t.Fatal(rr.Body.String())
	}
}

func TestHandlerFromFS_IndexCache(t *testing.T) {
	h := web.HandlerFromFS(fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(`<div id="root"></div>`)},
	})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != 200 {
		t.Fatal(rr.Code)
	}
	if rr.Header().Get("Cache-Control") != "no-cache" {
		t.Fatal(rr.Header().Get("Cache-Control"))
	}
}
