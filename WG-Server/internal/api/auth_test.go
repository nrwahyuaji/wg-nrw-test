package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"wgpanel/internal/auth"
	"wgpanel/internal/config"
	"wgpanel/internal/db"
	"wgpanel/internal/sqlc"
)

// testPassword hanya untuk DB sementara uji.
const testPassword = "kata-sandi-uji-panjang"

// testSecretKey = base64 dari 32 byte nol; hanya untuk uji.
const testSecretKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

// newTestApp membuat App di atas DB sementara bermigrasi, dengan user per role ber-password testPassword.
func newTestApp(t *testing.T) (*App, http.Handler) {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "uji.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := db.Migrate(context.Background(), database); err != nil {
		t.Fatal(err)
	}
	app, err := NewApp(database, config.Config{Environment: "development", SecretKey: testSecretKey})
	if err != nil {
		t.Fatal(err)
	}
	hash, err := auth.HashPassword(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"admin", "operator", "user"} {
		if _, err := app.Queries.CreateUser(context.Background(), sqlc.CreateUserParams{
			Username: role, PasswordHash: hash, Role: role, CreatedAt: 1, UpdatedAt: 1,
		}); err != nil {
			t.Fatal(err)
		}
	}
	return app, NewRouter(app)
}

// doRequest mengirim request ke router. Non-GET otomatis membawa header CSRF kecuali withCSRF=false.
func doRequest(router http.Handler, method, path, body string, cookie *http.Cookie, withCSRF bool) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if withCSRF {
		request.Header.Set("X-Requested-With", "wgpanel")
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

// loginAs login sebagai username dan mengembalikan cookie sesinya.
func loginAs(t *testing.T, router http.Handler, username, password string) *http.Cookie {
	t.Helper()
	response := doRequest(router, "POST", "/api/v1/auth/login",
		`{"username":"`+username+`","password":"`+password+`"}`, nil, true)
	if response.Code != http.StatusNoContent {
		t.Fatalf("login %s: status %d body %s", username, response.Code, response.Body)
	}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == auth.SessionCookieName {
			return cookie
		}
	}
	t.Fatal("login tidak memasang cookie sesi")
	return nil
}

// TestAuthLogin: benar 204 + cookie HttpOnly/SameSite=Strict, salah/username tidak ada/nonaktif 401 dengan pesan
// sama, field kosong 422, body rusak 400, percobaan ke-6 per menit 429 + Retry-After.
func TestAuthLogin(t *testing.T) {
	app, router := newTestApp(t)

	response := doRequest(router, "POST", "/api/v1/auth/login", `{"username":"Admin","password":"`+testPassword+`"}`, nil, true)
	if response.Code != http.StatusNoContent {
		t.Fatalf("login benar: %d %s", response.Code, response.Body)
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != auth.SessionCookieName || !cookies[0].HttpOnly ||
		cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].Path != "/" || cookies[0].MaxAge != 0 || cookies[0].Secure {
		t.Fatalf("cookie sesi salah: %+v", cookies)
	}

	if _, err := app.DB.Exec(`UPDATE users SET disabled = 1 WHERE username = 'user'`); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"password salah":     `{"username":"admin","password":"salah-salah-salah"}`,
		"username tidak ada": `{"username":"hantu","password":"` + testPassword + `"}`,
		"user dinonaktifkan": `{"username":"user","password":"` + testPassword + `"}`,
	} {
		response := doRequest(router, "POST", "/api/v1/auth/login", body, nil, true)
		if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), `"invalid_credentials"`) ||
			len(response.Result().Cookies()) != 0 {
			t.Fatalf("%s: harus 401 invalid_credentials tanpa cookie, got %d %s", name, response.Code, response.Body)
		}
	}

	if response := doRequest(router, "POST", "/api/v1/auth/login", `{"username":"admin"}`, nil, true); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("password kosong harus 422, got %d", response.Code)
	}
	if response := doRequest(router, "POST", "/api/v1/auth/login", `{rusak`, nil, true); response.Code != http.StatusBadRequest {
		t.Fatalf("body rusak harus 400, got %d", response.Code)
	}

	// Sejauh ini IP uji (192.0.2.1) memakai 4 dari 5 jatah per menit (422/400 tidak memakai jatah).
	if response := doRequest(router, "POST", "/api/v1/auth/login", `{"username":"admin","password":"salah-lagi-lagi"}`, nil, true); response.Code != http.StatusUnauthorized {
		t.Fatalf("percobaan ke-5 masih harus 401, got %d", response.Code)
	}
	response = doRequest(router, "POST", "/api/v1/auth/login", `{"username":"admin","password":"`+testPassword+`"}`, nil, true)
	if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") == "" ||
		!strings.Contains(response.Body.String(), `"rate_limited"`) {
		t.Fatalf("percobaan ke-6 per menit harus 429 + Retry-After, got %d %q %s",
			response.Code, response.Header().Get("Retry-After"), response.Body)
	}
}

// TestAuthCSRF: POST tanpa header X-Requested-With 403, Origin asing 403, Origin sama host / terdaftar lolos.
func TestAuthCSRF(t *testing.T) {
	app, _ := newTestApp(t)
	app.Config.AllowedOrigins = []string{"http://dev.example:5175"}
	router := NewRouter(app) // daftar origin dibaca saat router dibuat
	login := `{"username":"admin","password":"` + testPassword + `"}`

	if response := doRequest(router, "POST", "/api/v1/auth/login", login, nil, false); response.Code != http.StatusForbidden ||
		!strings.Contains(response.Body.String(), `"csrf_failed"`) {
		t.Fatalf("tanpa header CSRF harus 403, got %d %s", response.Code, response.Body)
	}
	// Setiap kasus memakai IP sendiri agar batas 5/menit per IP tidak ikut memengaruhi hasil.
	for index, testCase := range []struct {
		origin string
		want   int
	}{
		{"https://jahat.example", http.StatusForbidden},
		{"null", http.StatusForbidden},
		{"http://example.com", http.StatusNoContent}, // host request httptest = example.com
		{"http://dev.example:5175", http.StatusNoContent},
	} {
		request := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(login))
		request.Header.Set("X-Requested-With", "wgpanel")
		request.Header.Set("Origin", testCase.origin)
		request.RemoteAddr = "198.51.100." + strconv.Itoa(index+1) + ":1"
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != testCase.want {
			t.Fatalf("Origin %s: want %d got %d %s", testCase.origin, testCase.want, recorder.Code, recorder.Body)
		}
	}
	// GET tidak butuh header CSRF; header keamanan terpasang.
	response := doRequest(router, "GET", "/healthz", "", nil, false)
	if response.Code != http.StatusOK || response.Header().Get("Content-Security-Policy") == "" ||
		response.Header().Get("X-Content-Type-Options") != "nosniff" || response.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("healthz/header keamanan: %d %v", response.Code, response.Header())
	}
}

// TestAuthSessionFlow: /me tanpa cookie 401, dengan cookie 200 tanpa field rahasia, logout lalu /me 401,
// ganti password mencabut sesi lain (401) sementara sesi sendiri tetap 200, password lama salah 422,
// password baru pendek 422, dan password baru bisa dipakai login.
func TestAuthSessionFlow(t *testing.T) {
	_, router := newTestApp(t)

	if response := doRequest(router, "GET", "/api/v1/me", "", nil, false); response.Code != http.StatusUnauthorized {
		t.Fatalf("/me tanpa cookie harus 401, got %d", response.Code)
	}
	if response := doRequest(router, "GET", "/api/v1/me", "", &http.Cookie{Name: auth.SessionCookieName, Value: "palsu"}, false); response.Code != http.StatusUnauthorized {
		t.Fatalf("/me dengan token palsu harus 401, got %d", response.Code)
	}
	first := loginAs(t, router, "admin", testPassword)
	response := doRequest(router, "GET", "/api/v1/me", "", first, false)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"username":"admin"`) ||
		strings.Contains(response.Body.String(), "password") || strings.Contains(response.Body.String(), "totp") {
		t.Fatalf("/me dengan cookie: %d %s", response.Code, response.Body)
	}

	if response := doRequest(router, "POST", "/api/v1/auth/logout", "", first, true); response.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", response.Code)
	}
	if response := doRequest(router, "GET", "/api/v1/me", "", first, false); response.Code != http.StatusUnauthorized {
		t.Fatalf("/me setelah logout harus 401, got %d", response.Code)
	}

	current := loginAs(t, router, "admin", testPassword)
	other := loginAs(t, router, "admin", testPassword)
	if response := doRequest(router, "PUT", "/api/v1/me/password", `{"current_password":"salah-salah-salah","new_password":"password-baru-panjang"}`, current, true); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("password lama salah harus 422, got %d", response.Code)
	}
	if response := doRequest(router, "PUT", "/api/v1/me/password", `{"current_password":"`+testPassword+`","new_password":"pendek"}`, current, true); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("password baru pendek harus 422, got %d", response.Code)
	}
	if response := doRequest(router, "PUT", "/api/v1/me/password", `{"current_password":"`+testPassword+`","new_password":"password-baru-panjang"}`, current, true); response.Code != http.StatusNoContent {
		t.Fatalf("ganti password: %d %s", response.Code, response.Body)
	}
	if response := doRequest(router, "GET", "/api/v1/me", "", other, false); response.Code != http.StatusUnauthorized {
		t.Fatalf("sesi lain setelah ganti password harus 401, got %d", response.Code)
	}
	if response := doRequest(router, "GET", "/api/v1/me", "", current, false); response.Code != http.StatusOK {
		t.Fatalf("sesi sendiri harus tetap hidup, got %d", response.Code)
	}
	loginAs(t, router, "admin", "password-baru-panjang")
}
