// Package api: berkas ini berisi handler autentikasi: login, logout, profil sendiri, dan ganti password.
package api

import (
	"database/sql"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"wgpanel/internal/audit"
	"wgpanel/internal/auth"
	"wgpanel/internal/sqlc"
)

// maxUserAgentBytes memotong User-Agent yang disimpan di sesi agar klien tidak bisa mengisi DB dengan string raksasa.
const maxUserAgentBytes = 256

// userView adalah bentuk user di respons API. Struct terpisah dari baris DB supaya kolom rahasia
// (password_hash, totp_secret) tidak mungkin ikut terkirim.
type userView struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

// newUserView menyalin field publik user.
func newUserView(user sqlc.User) userView {
	return userView{ID: user.ID, Username: user.Username, Role: user.Role}
}

// login: POST /api/v1/auth/login {username, password}. 204 + cookie sesi bila benar; 401 invalid_credentials
// bila salah, username tidak ada, atau user nonaktif (satu pesan untuk semua, agar tidak membocorkan username);
// 429 rate_limited + Retry-After bila batas IP/username terlampaui; 422 bila field kosong.
func (app *App) login(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	body.Username = strings.TrimSpace(body.Username)
	if body.Username == "" || body.Password == "" {
		writeError(writer, http.StatusUnprocessableEntity, "validation_failed", "username dan password wajib diisi")
		return
	}
	ip := ClientIP(request, app.Config.TrustedProxies)
	if allowed, retryAfter := app.Limiter.Allow(ip, body.Username); !allowed {
		writer.Header().Set("Retry-After", strconv.Itoa(int(math.Max(1, math.Ceil(retryAfter.Seconds())))))
		writeError(writer, http.StatusTooManyRequests, "rate_limited", "terlalu banyak percobaan login, coba lagi nanti")
		return
	}
	user, err := app.Queries.GetUserByUsername(request.Context(), body.Username)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeInternalError(writer, request, err)
		return
	}
	// Username tidak ada tetap menjalankan bcrypt (hash dummy) agar waktu respons sama (DESIGN 7.3).
	// User nonaktif diperiksa SESUDAH password supaya waktunya juga sama.
	valid := false
	if err == nil {
		valid = auth.CheckPassword(user.PasswordHash, body.Password) && user.Disabled == 0
	} else {
		auth.CheckDummyPassword(body.Password)
	}
	if !valid {
		app.Limiter.RecordFailure(body.Username)
		actor := audit.Actor{Username: body.Username}
		if err == nil {
			actor.UserID = user.ID
		}
		app.recordAudit(request, actor, audit.ActionLoginFailed, "", map[string]any{"username": body.Username})
		writeError(writer, http.StatusUnauthorized, "invalid_credentials", "username atau password salah")
		return
	}
	app.Limiter.RecordSuccess(body.Username)
	app.recordAudit(request, audit.Actor{UserID: user.ID, Username: user.Username}, audit.ActionLogin, "", nil)
	// Fase 14: user ber-2FA mendapat stage totp_pending di sini.
	token, err := app.Sessions.Create(request.Context(), user.ID, auth.StageFull, ip, truncateUserAgent(request.UserAgent()))
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	http.SetCookie(writer, auth.SessionCookie(token, app.Config.IsProduction()))
	writer.WriteHeader(http.StatusNoContent)
}

// logout: POST /api/v1/auth/logout. Mencabut sesi cookie (bila ada) dan menghapus cookie. Selalu 204,
// juga tanpa sesi, supaya tombol logout tidak pernah gagal di sisi pengguna.
func (app *App) logout(writer http.ResponseWriter, request *http.Request) {
	if cookie, err := request.Cookie(auth.SessionCookieName); err == nil && cookie.Value != "" {
		if session, user, err := app.Sessions.Lookup(request.Context(), cookie.Value); err == nil {
			app.recordAudit(request, audit.Actor{UserID: user.ID, Username: user.Username}, audit.ActionLogout, "", nil)
			_ = session
		}
		if err := app.Sessions.Delete(request.Context(), cookie.Value); err != nil {
			writeInternalError(writer, request, err)
			return
		}
	}
	http.SetCookie(writer, auth.ClearSessionCookie(app.Config.IsProduction()))
	writer.WriteHeader(http.StatusNoContent)
}

// me: GET /api/v1/me → profil user yang login.
func (app *App) me(writer http.ResponseWriter, request *http.Request) {
	writeData(writer, http.StatusOK, newUserView(currentSession(request).User))
}

// changePassword: PUT /api/v1/me/password {current_password, new_password}. Password lama wajib benar (422 bila
// salah, bukan 401, agar frontend tidak mengira sesi habis). Password baru divalidasi HashPassword (422).
// Sukses: semua sesi lain user ini dicabut, sesi yang dipakai tetap hidup; 204.
func (app *App) changePassword(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	current := currentSession(request)
	if !auth.CheckPassword(current.User.PasswordHash, body.CurrentPassword) {
		writeError(writer, http.StatusUnprocessableEntity, "invalid_current_password", "password lama salah")
		return
	}
	hash, err := auth.HashPassword(body.NewPassword)
	if errors.Is(err, auth.ErrPasswordTooShort) || errors.Is(err, auth.ErrPasswordTooLong) {
		writeError(writer, http.StatusUnprocessableEntity, "validation_failed", err.Error())
		return
	}
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	ctx := request.Context()
	err = app.Queries.UpdateUserPassword(ctx, sqlc.UpdateUserPasswordParams{
		PasswordHash: hash, UpdatedAt: time.Now().Unix(), ID: current.User.ID,
	})
	if err == nil {
		err = app.Sessions.DeleteOthers(ctx, current.User.ID, current.Session.TokenHash)
	}
	if err == nil {
		app.recordAudit(request, audit.Actor{UserID: current.User.ID, Username: current.User.Username},
			audit.ActionPasswordChange, target("user", current.User.ID), nil)
	}
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

// truncateUserAgent memotong User-Agent ke 256 byte tanpa memecah karakter UTF-8.
func truncateUserAgent(userAgent string) string {
	if len(userAgent) <= maxUserAgentBytes {
		return userAgent
	}
	return strings.ToValidUTF8(userAgent[:maxUserAgentBytes], "")
}
