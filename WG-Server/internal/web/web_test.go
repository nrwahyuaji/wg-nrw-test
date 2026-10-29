package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// testFS meniru isi dist/ hasil build.
var testFS = fstest.MapFS{
	"index.html":       {Data: []byte("<!doctype html><title>WG-WebUI</title>")},
	"assets/x.js":      {Data: []byte("console.log(1)")},
	"assets/style.css": {Data: []byte("body{}")},
}

// serve menjalankan satu GET ke Handler.
func serve(handler http.Handler, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest("GET", path, nil))
	return recorder
}

// TestHandler (PLAN 6.1): / dan /peers/12 → index.html no-cache, /assets/x.js → cache panjang, /api/nope → 404
// JSON bukan index, berkas tidak ada → index.html (rute klien), path traversal tetap di dalam dist.
func TestHandler(t *testing.T) {
	handler := Handler(testFS)
	for _, path := range []string{"/", "/peers/12", "/index.html", "/tidak/ada.png", "/../../etc/passwd"} {
		response := serve(handler, path)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "<title>WG-WebUI</title>") ||
			response.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("%s: harus index.html no-cache, got %d %q %q", path, response.Code, response.Header().Get("Cache-Control"), response.Body)
		}
	}
	asset := serve(handler, "/assets/x.js")
	if asset.Code != http.StatusOK || asset.Body.String() != "console.log(1)" || asset.Header().Get("Cache-Control") != assetCacheControl {
		t.Fatalf("aset: %d %q %q", asset.Code, asset.Header().Get("Cache-Control"), asset.Body)
	}
	for _, path := range []string{"/api/nope", "/api", "/api/v1/interfaces"} {
		response := serve(handler, path)
		if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), "<title>") ||
			!strings.HasPrefix(response.Header().Get("Content-Type"), "application/json") {
			t.Errorf("%s: harus 404 JSON, got %d %q", path, response.Code, response.Body)
		}
	}
}

// TestMissingBuild: dist kosong (biner tanpa `make web`) → halaman penjelas, bukan 500/panic.
func TestMissingBuild(t *testing.T) {
	response := serve(Handler(fstest.MapFS{}), "/")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "make web") {
		t.Fatalf("dist kosong: %d %q", response.Code, response.Body)
	}
}

// TestEmbedded: embed asli tidak panic dan selalu menjawab (isi tergantung sudah `make web` atau belum).
func TestEmbedded(t *testing.T) {
	if response := serve(Handler(nil), "/"); response.Code != http.StatusOK {
		t.Fatalf("embed: %d", response.Code)
	}
}
