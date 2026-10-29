package api

import (
	"net/http"
	"testing"
)

// TestAccessMatrix: satu tabel izin untuk setiap endpoint (DESIGN 7.4). Setiap endpoint baru WAJIB menambah
// barisnya di sini pada commit yang sama (AGENTS.md). Role "" = tanpa sesi. Fase 13 melengkapi tabel ini
// dengan pengelompokan requireRole.
func TestAccessMatrix(t *testing.T) {
	_, router, _ := newInterfaceTestApp(t)
	cookies := map[string]*http.Cookie{}
	for _, role := range []string{"admin", "operator", "user"} {
		cookies[role] = loginAs(t, router, role, testPassword)
	}
	rows := []struct {
		method, path, role string
		want               int
	}{
		{"GET", "/healthz", "", http.StatusOK},
		{"GET", "/readyz", "", http.StatusOK},
		{"POST", "/api/v1/auth/logout", "", http.StatusNoContent},
		{"GET", "/api/v1/me", "", http.StatusUnauthorized},
		{"GET", "/api/v1/me", "admin", http.StatusOK},
		{"GET", "/api/v1/me", "operator", http.StatusOK},
		{"GET", "/api/v1/me", "user", http.StatusOK},
		// Body kosong: lolos izin lalu ditolak validasi (400), membuktikan role mana pun boleh memanggilnya.
		{"PUT", "/api/v1/me/password", "", http.StatusUnauthorized},
		{"PUT", "/api/v1/me/password", "admin", http.StatusBadRequest},
		{"PUT", "/api/v1/me/password", "operator", http.StatusBadRequest},
		{"PUT", "/api/v1/me/password", "user", http.StatusBadRequest},
		{"GET", "/api/v1/tidak-ada", "admin", http.StatusNotFound},
		// Interface (Fase 4): baca admin+operator, tulis admin. Body kosong → 400 membuktikan izin lolos.
		{"GET", "/api/v1/interfaces", "", http.StatusUnauthorized},
		{"GET", "/api/v1/interfaces", "admin", http.StatusOK},
		{"GET", "/api/v1/interfaces", "operator", http.StatusOK},
		{"GET", "/api/v1/interfaces", "user", http.StatusForbidden},
		{"GET", "/api/v1/interfaces/1", "admin", http.StatusNotFound},
		{"GET", "/api/v1/interfaces/1", "operator", http.StatusNotFound},
		{"GET", "/api/v1/interfaces/1", "user", http.StatusForbidden},
		{"POST", "/api/v1/interfaces", "", http.StatusUnauthorized},
		{"POST", "/api/v1/interfaces", "admin", http.StatusBadRequest},
		{"POST", "/api/v1/interfaces", "operator", http.StatusForbidden},
		{"POST", "/api/v1/interfaces", "user", http.StatusForbidden},
		{"GET", "/api/v1/interfaces/1/health", "", http.StatusUnauthorized},
		{"GET", "/api/v1/interfaces/1/health", "admin", http.StatusNotFound},
		{"GET", "/api/v1/interfaces/1/health", "operator", http.StatusNotFound},
		{"GET", "/api/v1/interfaces/1/health", "user", http.StatusForbidden},
		// Peer (Fase 5): baca admin+operator, tulis admin. id 99 tidak ada → 404 membuktikan izin lolos.
		{"GET", "/api/v1/peers", "", http.StatusUnauthorized},
		{"GET", "/api/v1/peers", "admin", http.StatusOK},
		{"GET", "/api/v1/peers", "operator", http.StatusOK},
		{"GET", "/api/v1/peers", "user", http.StatusForbidden},
		{"GET", "/api/v1/peers/99", "admin", http.StatusNotFound},
		{"GET", "/api/v1/peers/99", "operator", http.StatusNotFound},
		{"GET", "/api/v1/peers/99", "user", http.StatusForbidden},
		{"POST", "/api/v1/peers", "", http.StatusUnauthorized},
		{"POST", "/api/v1/peers", "admin", http.StatusBadRequest},
		{"POST", "/api/v1/peers", "operator", http.StatusForbidden},
		{"POST", "/api/v1/peers", "user", http.StatusForbidden},
		{"PUT", "/api/v1/peers/99", "admin", http.StatusNotFound},
		{"PUT", "/api/v1/peers/99", "operator", http.StatusForbidden},
		{"PUT", "/api/v1/peers/99", "user", http.StatusForbidden},
		{"DELETE", "/api/v1/peers/99", "admin", http.StatusNotFound},
		{"DELETE", "/api/v1/peers/99", "operator", http.StatusForbidden},
		{"DELETE", "/api/v1/peers/99", "user", http.StatusForbidden},
		{"POST", "/api/v1/peers/99/enable", "admin", http.StatusNotFound},
		{"POST", "/api/v1/peers/99/enable", "operator", http.StatusForbidden},
		{"POST", "/api/v1/peers/99/enable", "user", http.StatusForbidden},
		{"POST", "/api/v1/peers/99/disable", "admin", http.StatusNotFound},
		{"POST", "/api/v1/peers/99/disable", "operator", http.StatusForbidden},
		{"POST", "/api/v1/peers/99/disable", "user", http.StatusForbidden},
		{"GET", "/api/v1/peers/99/config", "", http.StatusUnauthorized},
		{"GET", "/api/v1/peers/99/config", "admin", http.StatusNotFound},
		{"GET", "/api/v1/peers/99/config", "operator", http.StatusForbidden},
		{"GET", "/api/v1/peers/99/config", "user", http.StatusForbidden},
		{"GET", "/api/v1/peers/99/qr", "", http.StatusUnauthorized},
		{"GET", "/api/v1/peers/99/qr", "admin", http.StatusNotFound},
		{"GET", "/api/v1/peers/99/qr", "operator", http.StatusForbidden},
		{"GET", "/api/v1/peers/99/qr", "user", http.StatusForbidden},
		// Trafik (Fase 8): baca admin+operator.
		{"GET", "/api/v1/peers/99/traffic", "", http.StatusUnauthorized},
		{"GET", "/api/v1/peers/99/traffic", "admin", http.StatusNotFound},
		{"GET", "/api/v1/peers/99/traffic", "operator", http.StatusNotFound},
		{"GET", "/api/v1/peers/99/traffic", "user", http.StatusForbidden},
		{"GET", "/api/v1/interfaces/1/traffic", "", http.StatusUnauthorized},
		{"GET", "/api/v1/interfaces/1/traffic", "admin", http.StatusNotFound},
		{"GET", "/api/v1/interfaces/1/traffic", "operator", http.StatusNotFound},
		{"GET", "/api/v1/interfaces/1/traffic", "user", http.StatusForbidden},
		{"GET", "/api/v1/status", "", http.StatusUnauthorized},
		{"GET", "/api/v1/status", "admin", http.StatusOK},
		{"GET", "/api/v1/status", "operator", http.StatusOK},
		{"GET", "/api/v1/status", "user", http.StatusForbidden},
		{"GET", "/api/v1/dashboard", "", http.StatusUnauthorized},
		{"GET", "/api/v1/dashboard", "admin", http.StatusOK},
		{"GET", "/api/v1/dashboard", "operator", http.StatusOK},
		{"GET", "/api/v1/dashboard", "user", http.StatusForbidden},
		{"PUT", "/api/v1/interfaces/1", "admin", http.StatusNotFound},
		{"PUT", "/api/v1/interfaces/1", "operator", http.StatusForbidden},
		{"PUT", "/api/v1/interfaces/1", "user", http.StatusForbidden},
	}
	for _, row := range rows {
		response := doRequest(router, row.method, row.path, "", cookies[row.role], true)
		if response.Code != row.want {
			t.Errorf("%s %s sebagai %q: want %d got %d", row.method, row.path, row.role, row.want, response.Code)
		}
	}
	// POST /auth/login publik: kasusnya ada di TestAuthLogin (204/401/422/429).
}
