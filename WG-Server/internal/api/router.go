// Package api: berkas ini merangkai router chi dan urutan middleware (PLAN Tugas 3.4).
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"wgpanel/internal/web"
)

// NewRouter memasang semua rute. Urutan middleware: Recoverer → requestLog → securityHeaders → csrfGuard,
// lalu requireSession per grup. Setiap endpoint baru wajib ditambahkan ke access_test.go.
func NewRouter(app *App) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.Recoverer, requestLog, securityHeaders, csrfGuard(app.Config.AllowedOrigins))
	// Path yang tidak cocok rute API: GET/HEAD → SPA ter-embed (rute sisi klien + aset; /api/* tetap 404 JSON
	// di dalam web.Handler), metode lain → 404 JSON.
	spa := web.Handler(nil)
	router.NotFound(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet || request.Method == http.MethodHead {
			spa.ServeHTTP(writer, request)
			return
		}
		writeError(writer, http.StatusNotFound, "not_found", "alamat tidak ditemukan")
	})
	router.MethodNotAllowed(func(writer http.ResponseWriter, _ *http.Request) {
		writeError(writer, http.StatusMethodNotAllowed, "method_not_allowed", "metode tidak didukung")
	})

	// /healthz = proses hidup; /readyz = DB menjawab. Tanpa auth (dipakai pemantau).
	router.Get("/healthz", func(writer http.ResponseWriter, _ *http.Request) {
		writeData(writer, http.StatusOK, map[string]string{"status": "ok"})
	})
	router.Get("/readyz", func(writer http.ResponseWriter, request *http.Request) {
		if err := app.DB.PingContext(request.Context()); err != nil {
			writeError(writer, http.StatusServiceUnavailable, "db_unavailable", "basis data tidak menjawab")
			return
		}
		writeData(writer, http.StatusOK, map[string]string{"status": "ready"})
	})

	router.Route("/api/v1", func(api chi.Router) {
		api.Post("/auth/login", app.login)
		api.Post("/auth/logout", app.logout)
		// publik share (token = izin), tanpa sesi dan tanpa CSRF preflight masalah? tetap butuh header X-Requested-With
		// untuk non-GET; GET jadi aman.
		api.Get("/share/{token}", app.shareViewPublic)
		api.Get("/share/{token}/config", app.shareConfigPublic)
		api.Get("/share/{token}/qr", app.shareQRPublic)
		api.Group(func(private chi.Router) {
			private.Use(app.requireSession)
			private.Get("/me", app.me)
			private.Put("/me/password", app.changePassword)

			// Baca: admin + operator (tanpa field rahasia). Tulis: admin saja (DESIGN 7.4).
			private.With(requireRole("admin", "operator")).Get("/interfaces", app.listInterfaces)
			private.With(requireRole("admin", "operator")).Get("/interfaces/{id}", app.getInterface)
			private.With(requireRole("admin", "operator")).Get("/interfaces/{id}/health", app.interfaceHealth)
			private.With(requireRole("admin", "operator")).Get("/interfaces/{id}/traffic", app.interfaceTraffic)
			private.With(requireRole("admin")).Post("/interfaces", app.createInterface)
			private.With(requireRole("admin")).Put("/interfaces/{id}", app.updateInterface)

			private.With(requireRole("admin", "operator")).Get("/status", app.peerStatus)
			private.With(requireRole("admin", "operator")).Get("/dashboard", app.dashboard)
			private.With(requireRole("admin", "operator")).Get("/peers", app.listPeers)
			private.With(requireRole("admin", "operator")).Get("/peers/{id}", app.getPeer)
			private.With(requireRole("admin", "operator")).Get("/peers/{id}/traffic", app.peerTraffic)
			private.With(requireRole("admin")).Post("/peers", app.createPeer)
			private.With(requireRole("admin")).Put("/peers/{id}", app.updatePeer)
			private.With(requireRole("admin")).Delete("/peers/{id}", app.deletePeer)
			private.With(requireRole("admin")).Post("/peers/{id}/enable", app.enablePeer)
			private.With(requireRole("admin")).Post("/peers/{id}/disable", app.disablePeer)
			// Config & QR memuat private key client: admin saja (DESIGN 7.4; pemilik lewat portal Fase 17).
			private.With(requireRole("admin")).Get("/peers/{id}/config", app.peerConfig)
			private.With(requireRole("admin")).Get("/peers/{id}/qr", app.peerQR)
			private.With(requireRole("admin")).Post("/peers/{id}/share", app.createShare)
			private.With(requireRole("admin")).Get("/peers/{id}/shares", app.listShares)
			private.With(requireRole("admin")).Delete("/shares/{id}", app.revokeShare)
			// Audit (Fase 10): admin saja.
			private.With(requireRole("admin")).Get("/audit", app.listAudit)
			private.With(requireRole("admin")).Get("/audit.csv", app.auditCSV)
			private.With(requireRole("admin")).Post("/backup", app.backup)
		})
	})
	return router
}
