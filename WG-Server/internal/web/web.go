// Package web menyajikan SPA React hasil build yang di-embed ke biner (DESIGN 11), sehingga VPS tidak butuh
// Node maupun nginx khusus frontend. Isi dist/ disalin dari WG-Frontend/dist oleh `make web`; di repo hanya ada
// dist/.gitkeep, jadi biner yang dibangun tanpa `make web` menyajikan halaman penjelas, bukan galat.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var embedded embed.FS

// assetCacheControl: berkas di assets/ ber-hash konten (nama berubah tiap build), jadi aman di-cache selamanya.
const assetCacheControl = "max-age=31536000, immutable"

// missingBuildPage tampil bila dist/ belum berisi hasil build (biner dev tanpa `make web`).
const missingBuildPage = `<!doctype html><html lang="id"><meta charset="utf-8"><title>WG-WebUI</title>` +
	`<p>Frontend belum dibangun. Jalankan <code>make web</code> lalu build ulang biner.</p></html>`

// Handler menyajikan SPA: berkas yang ada → disajikan apa adanya; /api/* → 404 JSON (bukan index.html, supaya
// klien API tidak salah membaca HTML sebagai data); path lain → index.html (rute sisi klien).
// fsys = nil memakai hasil embed; uji mengisinya dengan fstest.MapFS.
func Handler(fsys fs.FS) http.Handler {
	if fsys == nil {
		sub, err := fs.Sub(embedded, "dist")
		if err != nil {
			panic(err) // hanya mungkin bila direktif embed berubah
		}
		fsys = sub
	}
	files := http.FileServer(http.FS(fsys))
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api" || strings.HasPrefix(request.URL.Path, "/api/") {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte(`{"error":{"code":"not_found","message":"alamat tidak ditemukan"}}`))
			return
		}
		name := strings.TrimPrefix(path.Clean(request.URL.Path), "/")
		if name != "" && name != "index.html" {
			if info, err := fs.Stat(fsys, name); err == nil && !info.IsDir() {
				if strings.HasPrefix(name, "assets/") {
					writer.Header().Set("Cache-Control", assetCacheControl)
				}
				files.ServeHTTP(writer, request)
				return
			}
		}
		serveIndex(writer, fsys)
	})
}

// serveIndex mengirim index.html dengan no-cache, supaya build baru langsung terpakai setelah biner diganti.
func serveIndex(writer http.ResponseWriter, fsys fs.FS) {
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-cache")
	content, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		_, _ = writer.Write([]byte(missingBuildPage))
		return
	}
	_, _ = writer.Write(content)
}
