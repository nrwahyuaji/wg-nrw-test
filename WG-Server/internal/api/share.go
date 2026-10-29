// Package api: tautan berbagi sekali pakai (PLAN 10.2, PRD F-4.3, N-9). Khusus admin.
// Token asli hanya lewat URL sekali; server hanya menyimpan hash.
package api

import (
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/skip2/go-qrcode"

	"wgpanel/internal/audit"
	"wgpanel/internal/auth"
	"wgpanel/internal/peer"
	"wgpanel/internal/sqlc"
	"wgpanel/internal/wgconf"
)

// ShareView adalah respons POST /peers/{id}/share.
type shareView struct {
	URL       string `json:"url"`
	ExpiresAt int64  `json:"expires_at"`
}

// shareDefaultHours bawaan bila setting share_default_hours tidak ada/tidak valid.
const shareDefaultHours = 24

// shareMaxHours = 168 (7 hari) sesuai PLAN 10.2.
const shareMaxHours = 168

// shareOpenWindow: /share/{token}/config|qr hanya berlaku 10 menit setelah dibuka.
const shareOpenWindow = 10 * time.Minute

// createShare: POST /peers/{id}/share {hours?}. Admin. → {url, expires_at}.
func (app *App) createShare(writer http.ResponseWriter, request *http.Request) {
	current, ok := app.loadPeer(writer, request)
	if !ok {
		return
	}
	var body struct {
		Hours int64 `json:"hours"`
	}
	// Body opsional: default hours dari setting.
	if request.Body != nil && request.ContentLength != 0 {
		if !decodeJSON(writer, request, &body) {
			return
		}
	}
	hours := body.Hours
	if hours == 0 {
		hours = app.shareDefaultHours(request)
	}
	if hours < 1 || hours > shareMaxHours {
		writeValidation(writer, []fieldError{{Field: "hours", Message: "hours harus 1–168"}})
		return
	}
	plain, hash, err := auth.NewToken()
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	now := time.Now().Unix()
	expires := now + hours*3600
	userID := currentSession(request).User.ID
	link, err := app.Queries.CreateShareLink(request.Context(), sqlc.CreateShareLinkParams{
		PeerID: current.ID, TokenHash: hash, CreatedBy: nullableID(&userID),
		CreatedAt: now, ExpiresAt: expires,
	})
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	app.recordAudit(request, audit.Actor{UserID: userID, Username: currentSession(request).User.Username},
		"share.create", target("peer", current.ID), map[string]any{"hours": hours, "expires_at": expires})
	writeData(writer, http.StatusCreated, shareView{URL: "/s/" + plain, ExpiresAt: link.ExpiresAt})
}

// shareDefaultHours membaca setting share_default_hours (fallback 24).
func (app *App) shareDefaultHours(request *http.Request) int64 {
	if value, err := app.Queries.GetSetting(request.Context(), "share_default_hours"); err == nil {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil && parsed >= 1 && parsed <= shareMaxHours {
			return parsed
		}
	}
	return shareDefaultHours
}

// listShares: GET /peers/{id}/shares (admin) → daftar tautan peer ini (tanpa token).
func (app *App) listShares(writer http.ResponseWriter, request *http.Request) {
	current, ok := app.loadPeer(writer, request)
	if !ok {
		return
	}
	rows, err := app.Queries.ListShareLinksByPeer(request.Context(), current.ID)
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	type view struct {
		ID        int64  `json:"id"`
		CreatedAt int64  `json:"created_at"`
		ExpiresAt int64  `json:"expires_at"`
		UsedAt    *int64 `json:"used_at"`
		RevokedAt *int64 `json:"revoked_at"`
	}
	views := make([]view, 0, len(rows))
	for _, row := range rows {
		v := view{ID: row.ID, CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt}
		if row.UsedAt.Valid {
			v.UsedAt = &row.UsedAt.Int64
		}
		if row.RevokedAt.Valid {
			v.RevokedAt = &row.RevokedAt.Int64
		}
		views = append(views, v)
	}
	writeData(writer, http.StatusOK, views)
}

// revokeShare: DELETE /shares/{id} (admin) → 204.
func (app *App) revokeShare(writer http.ResponseWriter, request *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(request, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(writer, http.StatusNotFound, "not_found", "tautan tidak ditemukan")
		return
	}
	link, err := app.Queries.GetShareLinkByID(request.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(writer, http.StatusNotFound, "not_found", "tautan tidak ditemukan")
		return
	}
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	if link.RevokedAt.Valid {
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	if err := app.Queries.RevokeShareLink(request.Context(), sqlc.RevokeShareLinkParams{RevokedAt: sql.NullInt64{Int64: time.Now().Unix(), Valid: true}, ID: link.ID}); err != nil {
		writeInternalError(writer, request, err)
		return
	}
	app.recordAudit(request, audit.Actor{UserID: currentSession(request).User.ID, Username: currentSession(request).User.Username},
		"share.revoke", target("peer", link.PeerID), map[string]any{"share_id": link.ID})
	writer.WriteHeader(http.StatusNoContent)
}

// shareViewPublic: GET /share/{token} (publik). Valid bila used_at IS NULL AND revoked_at IS NULL AND expires_at > now.
// Lalu menandai used_at secara atomik (0 baris = sudah dipakai → 410).
func (app *App) shareViewPublic(writer http.ResponseWriter, request *http.Request) {
	token := chi.URLParam(request, "token")
	link, err := app.Queries.GetShareLinkByHash(request.Context(), auth.HashToken(token))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(writer, http.StatusNotFound, "not_found", "tautan tidak ditemukan")
		return
	}
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	now := time.Now().Unix()
	if link.RevokedAt.Valid || link.UsedAt.Valid || link.ExpiresAt <= now {
		writeError(writer, http.StatusGone, "share_gone", "tautan sudah tidak berlaku")
		return
	}
	// Tandai used_at atomik.
	affected, err := app.Queries.MarkShareLinkUsed(request.Context(), sqlc.MarkShareLinkUsedParams{UsedAt: sql.NullInt64{Int64: now, Valid: true}, ID: link.ID})
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	if affected == 0 {
		writeError(writer, http.StatusGone, "share_gone", "tautan sudah tidak berlaku")
		return
	}
	// Render config + QR (data URL) sekali. Header no-store + no-referrer.
	row, err := app.Queries.GetPeer(request.Context(), link.PeerID)
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	iface, err := app.Queries.GetInterface(request.Context(), row.InterfaceID)
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	config, err := peer.BuildClientConfig(iface, row, app.Box)
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	content := wgconf.RenderClient(config)
	png, err := qrcode.Encode(content, qrcode.Medium, qrSizePixels)
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	qrData := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writeData(writer, http.StatusOK, map[string]any{
		"peer_name": row.Name, "config": content, "qr_data_url": qrData,
		"expires_at": link.ExpiresAt, "opened_at": now,
	})
}

// shareConfigPublic: GET /share/{token}/config → text/plain, hanya 10 menit setelah dibuka.
func (app *App) shareConfigPublic(writer http.ResponseWriter, request *http.Request) {
	link, row, ok := app.loadOpenShare(request, writer)
	if !ok {
		return
	}
	_ = link
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	// Konten di-render ulang dari DB (peer/interface masih ada).
	iface, err := app.Queries.GetInterface(request.Context(), row.InterfaceID)
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	config, err := peer.BuildClientConfig(iface, row, app.Box)
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	_, _ = writer.Write([]byte(wgconf.RenderClient(config)))
}

// shareQRPublic: GET /share/{token}/qr → image/png, hanya 10 menit setelah dibuka.
func (app *App) shareQRPublic(writer http.ResponseWriter, request *http.Request) {
	_, row, ok := app.loadOpenShare(request, writer)
	if !ok {
		return
	}
	iface, err := app.Queries.GetInterface(request.Context(), row.InterfaceID)
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	config, err := peer.BuildClientConfig(iface, row, app.Box)
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	png, err := qrcode.Encode(wgconf.RenderClient(config), qrcode.Medium, qrSizePixels)
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	writer.Header().Set("Content-Type", "image/png")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	_, _ = writer.Write(png)
}

// loadOpenShare memvalidasi tautan yang sudah dibuka: used_at tidak null, belum lewat 10 menit.
func (app *App) loadOpenShare(request *http.Request, writer http.ResponseWriter) (sqlc.ShareLink, sqlc.Peer, bool) {
	token := chi.URLParam(request, "token")
	link, err := app.Queries.GetShareLinkByHash(request.Context(), auth.HashToken(token))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(writer, http.StatusNotFound, "not_found", "tautan tidak ditemukan")
		return sqlc.ShareLink{}, sqlc.Peer{}, false
	}
	if err != nil {
		writeInternalError(writer, request, err)
		return sqlc.ShareLink{}, sqlc.Peer{}, false
	}
	if !link.UsedAt.Valid {
		writeError(writer, http.StatusForbidden, "share_not_opened", "tautan belum dibuka")
		return sqlc.ShareLink{}, sqlc.Peer{}, false
	}
	if time.Now().Unix() > link.UsedAt.Int64+int64(shareOpenWindow.Seconds()) {
		writeError(writer, http.StatusGone, "share_gone", "jendela unduh 10 menit sudah lewat")
		return sqlc.ShareLink{}, sqlc.Peer{}, false
	}
	row, err := app.Queries.GetPeer(request.Context(), link.PeerID)
	if err != nil {
		writeInternalError(writer, request, err)
		return sqlc.ShareLink{}, sqlc.Peer{}, false
	}
	return link, row, true
}
