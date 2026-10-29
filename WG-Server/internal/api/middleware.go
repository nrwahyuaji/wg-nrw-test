// Package api: berkas ini berisi middleware HTTP: log request, header keamanan, penjaga CSRF, dan sesi.
package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"wgpanel/internal/auth"
	"wgpanel/internal/sqlc"
)

// contentSecurityPolicy sesuai DESIGN 12: hanya sumber sendiri, gambar data: untuk QR, tidak boleh di-frame.
const contentSecurityPolicy = "default-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'"

// csrfHeaderValue: nilai wajib header X-Requested-With untuk request non-GET.
const csrfHeaderValue = "wgpanel"

// sessionContextKey menyimpan sesi+user yang lolos requireSession di context request.
type sessionContextKey struct{}

// authenticated adalah sesi dan user milik request yang sudah lolos requireSession.
type authenticated struct {
	Session sqlc.Session
	User    sqlc.User
}

// requestLog mencatat method, path, status, dan durasi setiap request. Body dan query string sengaja tidak
// dicatat karena bisa memuat password atau token.
func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		started := time.Now()
		wrapped := middleware.NewWrapResponseWriter(writer, request.ProtoMajor)
		next.ServeHTTP(wrapped, request)
		slog.Info("http", "method", request.Method, "path", request.URL.Path,
			"status", wrapped.Status(), "durasi_ms", time.Since(started).Milliseconds())
	})
}

// securityHeaders memasang CSP dan header pengerasan di semua respons.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		header := writer.Header()
		header.Set("Content-Security-Policy", contentSecurityPolicy)
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(writer, request)
	})
}

// csrfGuard (DESIGN 7.2): request yang mengubah state wajib membawa X-Requested-With: wgpanel. Header custom
// memaksa preflight CORS yang tidak pernah diizinkan server, jadi form/fetch lintas situs tidak bisa mengirimnya.
// Origin (bila dikirim peramban) juga harus sama dengan host request atau terdaftar di allowedOrigins.
// Host dibandingkan tanpa skema karena TLS bisa diakhiri di reverse proxy.
func csrfGuard(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(writer, request)
				return
			}
			if request.Header.Get("X-Requested-With") != csrfHeaderValue {
				writeError(writer, http.StatusForbidden, "csrf_failed", "permintaan ditolak: header X-Requested-With wajib")
				return
			}
			if origin := request.Header.Get("Origin"); origin != "" && !originAllowed(origin, request.Host, allowedOrigins) {
				writeError(writer, http.StatusForbidden, "csrf_failed", "permintaan ditolak: origin tidak diizinkan")
				return
			}
			next.ServeHTTP(writer, request)
		})
	}
}

// originAllowed: origin cocok bila host-nya sama dengan host request, atau string-nya terdaftar persis.
// Origin "null" (iframe sandbox, berkas lokal) tidak pernah cocok.
func originAllowed(origin, host string, allowedOrigins []string) bool {
	if slices.Contains(allowedOrigins, origin) {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && parsed.Host != "" && parsed.Host == host
}

// requireSession meloloskan hanya request dengan cookie sesi valid berstage full. Selain itu 401.
// Sesi dan user disimpan di context untuk handler (lihat currentSession).
func (app *App) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		cookie, err := request.Cookie(auth.SessionCookieName)
		if err != nil {
			writeError(writer, http.StatusUnauthorized, "unauthenticated", "silakan login")
			return
		}
		session, user, err := app.Sessions.Lookup(request.Context(), cookie.Value)
		if errors.Is(err, auth.ErrNoSession) || (err == nil && session.Stage != auth.StageFull) {
			writeError(writer, http.StatusUnauthorized, "unauthenticated", "sesi berakhir, silakan login lagi")
			return
		}
		if err != nil {
			writeInternalError(writer, request, err)
			return
		}
		ctx := context.WithValue(request.Context(), sessionContextKey{}, authenticated{Session: session, User: user})
		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}

// requireRole meloloskan hanya user dengan salah satu role yang disebut; selain itu 403 (DESIGN 7.4).
// Dipasang di belakang requireSession, jadi sesi selalu ada.
func requireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if !slices.Contains(roles, currentSession(request).User.Role) {
				writeError(writer, http.StatusForbidden, "forbidden", "akun Anda tidak berhak melakukan ini")
				return
			}
			next.ServeHTTP(writer, request)
		})
	}
}

// currentSession mengambil sesi yang dipasang requireSession. Hanya dipanggil dari handler di belakangnya.
func currentSession(request *http.Request) authenticated {
	value, _ := request.Context().Value(sessionContextKey{}).(authenticated)
	return value
}
