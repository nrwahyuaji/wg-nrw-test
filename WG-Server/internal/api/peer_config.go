// Package api: berkas ini berisi unduhan config client dan QR (PLAN Tugas 5.2, PRD F-4.1, F-4.2). Khusus admin:
// isinya memuat private key client. Pencatatan audit unduhan (F-4.4) menyusul di Fase 10.
package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/skip2/go-qrcode"

	"wgpanel/internal/audit"
	"wgpanel/internal/peer"
	"wgpanel/internal/wgconf"
)

// qrSizePixels: 512 px cukup tajam untuk dipindai dari layar laptop maupun dicetak.
const qrSizePixels = 512

// mustParseID membaca {id} dari path.
func mustParseID(request *http.Request) int64 {
	id, _ := strconv.ParseInt(chi.URLParam(request, "id"), 10, 64)
	return id
}

// peerConfig: GET /peers/{id}/config → text/plain sebagai lampiran <nama-aman>.conf.
func (app *App) peerConfig(writer http.ResponseWriter, request *http.Request) {
	content, name, ok := app.renderPeerConfig(writer, request)
	if !ok {
		return
	}
	row, _ := app.Queries.GetPeer(request.Context(), mustParseID(request))
	app.recordAudit(request, audit.Actor{UserID: currentSession(request).User.ID, Username: currentSession(request).User.Username},
		audit.ActionPeerConfigDownload, target("peer", row.ID), map[string]any{"name": row.Name})
	header := writer.Header()
	header.Set("Content-Type", "text/plain; charset=utf-8")
	header.Set("Content-Disposition", `attachment; filename="`+name+`.conf"`)
	header.Set("Cache-Control", "no-store")
	header.Set("Content-Length", strconv.Itoa(len(content)))
	_, _ = writer.Write([]byte(content)) // galat = klien memutus koneksi
}

// peerQR: GET /peers/{id}/qr → image/png berisi teks .conf yang sama persis.
func (app *App) peerQR(writer http.ResponseWriter, request *http.Request) {
	content, _, ok := app.renderPeerConfig(writer, request)
	if !ok {
		return
	}
	row, _ := app.Queries.GetPeer(request.Context(), mustParseID(request))
	app.recordAudit(request, audit.Actor{UserID: currentSession(request).User.ID, Username: currentSession(request).User.Username},
		audit.ActionPeerQRDownload, target("peer", row.ID), map[string]any{"name": row.Name})
	png, err := qrcode.Encode(content, qrcode.Medium, qrSizePixels)
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	writer.Header().Set("Content-Type", "image/png")
	writer.Header().Set("Cache-Control", "no-store")
	_, _ = writer.Write(png)
}

// renderPeerConfig memuat peer + interface-nya lalu merender .conf. ok=false bila respons galat sudah dikirim.
func (app *App) renderPeerConfig(writer http.ResponseWriter, request *http.Request) (string, string, bool) {
	row, ok := app.loadPeer(writer, request)
	if !ok {
		return "", "", false
	}
	iface, err := app.Queries.GetInterface(request.Context(), row.InterfaceID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(writer, http.StatusNotFound, "not_found", "interface peer tidak ditemukan")
		return "", "", false
	}
	if err != nil {
		writeInternalError(writer, request, err)
		return "", "", false
	}
	config, err := peer.BuildClientConfig(iface, row, app.Box)
	if err != nil {
		writeInternalError(writer, request, err)
		return "", "", false
	}
	return wgconf.RenderClient(config), peer.SafeFileName(row.Name), true
}
