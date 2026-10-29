// Package api: berkas ini berisi handler peer (PLAN Tugas 5.1, PRD F-3.1–F-3.6, F-3.8). Semua penulisan lewat
// Reconciler.WithApply, sehingga peer baru/berubah/dihapus langsung terlihat di kernel atau tidak sama sekali.
package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"wgpanel/internal/audit"
	"wgpanel/internal/peer"
	"wgpanel/internal/reconcile"
	"wgpanel/internal/sqlc"
)

// target membentuk kolom target audit: "peer:12".
func target(kind string, id int64) string {
	return kind + ":" + strconv.FormatInt(id, 10)
}

// peerRequest adalah body POST/PUT /peers. InterfaceID dan UsePSK hanya dipakai POST (pindah interface dan
// tambah/cabut PSK bukan bagian PUT; rotasi = Fase 12).
type peerRequest struct {
	InterfaceID      int64    `json:"interface_id"`
	Name             string   `json:"name"`
	Note             string   `json:"note"`
	OwnerUserID      *int64   `json:"owner_user_id"`
	Address          string   `json:"address"`
	PublicKey        string   `json:"public_key"`
	UsePSK           bool     `json:"use_psk"`
	ClientAllowedIPs []string `json:"client_allowed_ips"`
	SiteSubnets      []string `json:"site_subnets"`
}

// peerView adalah bentuk peer di respons. TIDAK punya private key maupun PSK (N-1); hanya penanda boolean.
type peerView struct {
	ID               int64    `json:"id"`
	InterfaceID      int64    `json:"interface_id"`
	OwnerUserID      *int64   `json:"owner_user_id"`
	Name             string   `json:"name"`
	Note             string   `json:"note"`
	Address          string   `json:"address"`
	PublicKey        string   `json:"public_key"`
	HasPrivateKey    bool     `json:"has_private_key"`
	HasPSK           bool     `json:"has_psk"`
	ClientAllowedIPs []string `json:"client_allowed_ips"`
	SiteSubnets      []string `json:"site_subnets"`
	Enabled          bool     `json:"enabled"`
	DisabledReason   string   `json:"disabled_reason"`
	ExpiresAt        *int64   `json:"expires_at"`
	CreatedAt        int64    `json:"created_at"`
	UpdatedAt        int64    `json:"updated_at"`
}

// newPeerView menyalin field publik baris peer.
func newPeerView(row sqlc.Peer) peerView {
	view := peerView{
		ID: row.ID, InterfaceID: row.InterfaceID, Name: row.Name, Note: row.Note, Address: row.Address,
		PublicKey: row.PublicKey, HasPrivateKey: len(row.PrivateKey) > 0, HasPSK: len(row.PresharedKey) > 0,
		ClientAllowedIPs: splitCSV(row.ClientAllowedIps), SiteSubnets: splitCSV(row.SiteSubnets),
		Enabled: row.Enabled == 1, DisabledReason: row.DisabledReason, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	if row.OwnerUserID.Valid {
		view.OwnerUserID = &row.OwnerUserID.Int64
	}
	if row.ExpiresAt.Valid {
		view.ExpiresAt = &row.ExpiresAt.Int64
	}
	return view
}

// peerValidationError membawa hasil validasi keluar dari fn WithApply. Validasi dijalankan di dalam transaksi +
// mutex Reconciler supaya IP/subnet yang dicek tidak bisa direbut request lain sebelum commit.
type peerValidationError struct {
	fields   []peer.FieldError
	conflict bool
}

func (err *peerValidationError) Error() string { return "validasi peer gagal" }

// ErrStillExpired: enable peer yang status expired (tanggal belum diperpanjang).
var ErrStillExpired = errors.New("still_expired")

// ErrStillOverQuota: enable peer yang kuota belum dinaikkan / belum direset.
var ErrStillOverQuota = errors.New("still_over_quota")

// listPeers: GET /peers (admin, operator).
func (app *App) listPeers(writer http.ResponseWriter, request *http.Request) {
	rows, err := app.Queries.ListPeers(request.Context())
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	views := make([]peerView, 0, len(rows))
	for _, row := range rows {
		views = append(views, newPeerView(row))
	}
	writeData(writer, http.StatusOK, views)
}

// getPeer: GET /peers/{id} (admin, operator).
func (app *App) getPeer(writer http.ResponseWriter, request *http.Request) {
	row, ok := app.loadPeer(writer, request)
	if !ok {
		return
	}
	writeData(writer, http.StatusOK, newPeerView(row))
}

// createPeer: POST /peers (admin). Keypair dibuat server bila public_key kosong; PSK dibuat bila use_psk.
// 201 peerView; 422 validasi; 409 IP/public key sudah dipakai; 404 interface tidak ada.
func (app *App) createPeer(writer http.ResponseWriter, request *http.Request) {
	var body peerRequest
	if !decodeJSON(writer, request, &body) {
		return
	}
	ctx := request.Context()
	var created sqlc.Peer
	err := app.Reconciler.WithApply(ctx, body.InterfaceID, func(tx reconcile.Tx) (int64, error) {
		iface, err := tx.Queries.GetInterface(ctx, body.InterfaceID)
		if err != nil {
			return 0, err
		}
		input, err := validatePeer(ctx, tx.Queries, iface, 0, body)
		if err != nil {
			return 0, err
		}
		params := sqlc.CreatePeerParams{
			InterfaceID: iface.ID, OwnerUserID: nullableID(body.OwnerUserID), Name: input.Name, Note: input.Note,
			Address: input.Address, PublicKey: input.PublicKey, ClientAllowedIps: input.ClientAllowedIPs,
			SiteSubnets: input.SiteSubnets, CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix(),
		}
		if params.PublicKey == "" {
			if params.PublicKey, params.PrivateKey, err = app.sealedKeypair(); err != nil {
				return 0, err
			}
		}
		if body.UsePSK {
			if params.PresharedKey, err = app.sealedPSK(); err != nil {
				return 0, err
			}
		}
		created, err = tx.Queries.CreatePeer(ctx, params)
		if err != nil {
			return 0, err
		}
		return 0, app.auditTx(ctx, tx, request, audit.ActionPeerCreate, target("peer", created.ID),
			map[string]any{"name": created.Name, "address": created.Address, "interface_id": created.InterfaceID})
	})
	if err != nil {
		app.writePeerError(writer, request, err)
		return
	}
	writeData(writer, http.StatusCreated, newPeerView(created))
}

// updatePeer: PUT /peers/{id} (admin). Mengubah nama/catatan/owner/address/AllowedIPs/site_subnets. Key, PSK,
// dan interface tidak diubah di sini.
func (app *App) updatePeer(writer http.ResponseWriter, request *http.Request) {
	current, ok := app.loadPeer(writer, request)
	if !ok {
		return
	}
	var body peerRequest
	if !decodeJSON(writer, request, &body) {
		return
	}
	ctx := request.Context()
	var updated sqlc.Peer
	err := app.Reconciler.WithApply(ctx, current.InterfaceID, func(tx reconcile.Tx) (int64, error) {
		iface, err := tx.Queries.GetInterface(ctx, current.InterfaceID)
		if err != nil {
			return 0, err
		}
		body.PublicKey = "" // PUT tidak mengganti key; nilai yang dikirim diabaikan
		input, err := validatePeer(ctx, tx.Queries, iface, current.ID, body)
		if err != nil {
			return 0, err
		}
		updated, err = tx.Queries.UpdatePeer(ctx, sqlc.UpdatePeerParams{
			OwnerUserID: nullableID(body.OwnerUserID), Name: input.Name, Note: input.Note, Address: input.Address,
			ClientAllowedIps: input.ClientAllowedIPs, SiteSubnets: input.SiteSubnets, UpdatedAt: time.Now().Unix(),
			ID: current.ID,
		})
		if err != nil {
			return 0, err
		}
		// Hanya field yang berubah dicatat, supaya detail tetap kecil dan tidak memuat catatan bebas.
		changes := map[string]any{"name": updated.Name, "address": updated.Address}
		if current.Name != updated.Name {
			changes["name_before"] = current.Name
		}
		if current.Address != updated.Address {
			changes["address_before"] = current.Address
		}
		return 0, app.auditTx(ctx, tx, request, audit.ActionPeerUpdate, target("peer", updated.ID), changes)
	})
	if err != nil {
		app.writePeerError(writer, request, err)
		return
	}
	writeData(writer, http.StatusOK, newPeerView(updated))
}

// deletePeer: DELETE /peers/{id} (admin). 204; peer langsung hilang dari kernel.
func (app *App) deletePeer(writer http.ResponseWriter, request *http.Request) {
	current, ok := app.loadPeer(writer, request)
	if !ok {
		return
	}
	err := app.Reconciler.WithApply(request.Context(), current.InterfaceID, func(tx reconcile.Tx) (int64, error) {
		if err := tx.Queries.DeletePeer(request.Context(), current.ID); err != nil {
			return 0, err
		}
		return 0, app.auditTx(request.Context(), tx, request, audit.ActionPeerDelete, target("peer", current.ID),
			map[string]any{"name": current.Name, "address": current.Address, "interface_id": current.InterfaceID})
	})
	if err != nil {
		app.writePeerError(writer, request, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

// enablePeer / disablePeer: POST /peers/{id}/enable|disable (admin). Disable manual mencatat alasan 'manual'
// (F-3.6); enable mengosongkan alasan. Kedaluwarsa/kuota (Fase 9) memakai alasan lain lewat scheduler.
func (app *App) enablePeer(writer http.ResponseWriter, request *http.Request) {
	app.setPeerEnabled(writer, request, true)
}

func (app *App) disablePeer(writer http.ResponseWriter, request *http.Request) {
	app.setPeerEnabled(writer, request, false)
}

// setPeerEnabled menyalakan/mematikan peer lewat Reconciler. Bila peer sudah aktif (enabled=1), abaikan.
// Bila peer expired (tanggal belum diperpanjang) → 409 still_expired.
// Bila peer over quota (disabled_reason=quota, kuota belum dinaikkan/reset) → 409 still_over_quota.
func (app *App) setPeerEnabled(writer http.ResponseWriter, request *http.Request, enabled bool) {
	current, ok := app.loadPeer(writer, request)
	if !ok {
		return
	}
	if enabled && current.Enabled == 1 {
		writeData(writer, http.StatusOK, newPeerView(current))
		return
	}
	if enabled && current.DisabledReason == "expired" {
		writeError(writer, http.StatusConflict, "still_expired", "peer masih kedaluwarsa, perlu perpanjang dahulu")
		return
	}
	if enabled && current.DisabledReason == "quota" {
		writeError(writer, http.StatusConflict, "still_over_quota", "kuota peer belum dinaikkan atau belum di-reset")
		return
	}
	params := sqlc.SetPeerEnabledParams{Enabled: 0, DisabledReason: "manual", UpdatedAt: time.Now().Unix(), ID: current.ID}
	if enabled {
		params.Enabled, params.DisabledReason = 1, ""
	}
	var updated sqlc.Peer
	err := app.Reconciler.WithApply(request.Context(), current.InterfaceID, func(tx reconcile.Tx) (int64, error) {
		var err error
		updated, err = tx.Queries.SetPeerEnabled(request.Context(), params)
		if err != nil {
			return 0, err
		}
		action := audit.ActionPeerDisable
		if enabled {
			action = audit.ActionPeerEnable
		}
		return 0, app.auditTx(request.Context(), tx, request, action, target("peer", updated.ID),
			map[string]any{"name": updated.Name})
	})
	if err != nil {
		app.writePeerError(writer, request, err)
		return
	}
	writeData(writer, http.StatusOK, newPeerView(updated))
}

// validatePeer membangun konteks validasi dari DB (di dalam transaksi) lalu memanggil peer.Validate.
// excludeID = peer yang sedang diubah, supaya IP/subnet miliknya sendiri tidak dihitung bentrok.
func validatePeer(ctx context.Context, queries *sqlc.Queries, iface sqlc.Interface, excludeID int64, body peerRequest) (peer.Input, error) {
	server, err := netip.ParsePrefix(iface.Address)
	if err != nil {
		return peer.Input{}, err
	}
	addresses, err := queries.ListPeerAddresses(ctx, iface.ID)
	if err != nil {
		return peer.Input{}, err
	}
	used := make(map[netip.Addr]struct{}, len(addresses))
	for _, row := range addresses {
		if prefix, err := netip.ParsePrefix(row.Address); err == nil && row.ID != excludeID {
			used[prefix.Addr()] = struct{}{}
		}
	}
	sites, err := queries.ListSiteSubnets(ctx)
	if err != nil {
		return peer.Input{}, err
	}
	var others []netip.Prefix
	for _, row := range sites {
		if row.ID != excludeID {
			others = append(others, peer.ParseCIDRColumn(row.SiteSubnets)...)
		}
	}
	input, fields, conflict := peer.Validate(peer.Request{
		Name: body.Name, Note: body.Note, Address: body.Address, PublicKey: body.PublicKey,
		ClientAllowedIPs: body.ClientAllowedIPs, SiteSubnets: body.SiteSubnets,
	}, peer.Context{Server: server, UsedAddresses: used, OtherSites: others})
	if len(fields) > 0 {
		return peer.Input{}, &peerValidationError{fields: fields, conflict: conflict}
	}
	if body.OwnerUserID != nil {
		if _, err := queries.GetUserByID(ctx, *body.OwnerUserID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return peer.Input{}, &peerValidationError{fields: []peer.FieldError{{Field: "owner_user_id", Message: "user pemilik tidak ditemukan"}}}
			}
			return peer.Input{}, err
		}
	}
	return input, nil
}

// writePeerError memetakan galat penulisan peer ke respons HTTP.
func (app *App) writePeerError(writer http.ResponseWriter, request *http.Request, err error) {
	var validation *peerValidationError
	switch {
	case errors.As(err, &validation) && validation.conflict:
		writeError(writer, http.StatusConflict, "address_taken", validation.fields[0].Message)
	case errors.As(err, &validation):
		fields := make([]fieldError, 0, len(validation.fields))
		for _, field := range validation.fields {
			fields = append(fields, fieldError(field))
		}
		writeValidation(writer, fields)
	case errors.Is(err, sql.ErrNoRows):
		writeError(writer, http.StatusNotFound, "not_found", "interface tidak ditemukan")
	case errors.Is(err, ErrStillExpired):
		writeError(writer, http.StatusConflict, "still_expired", "peer masih kedaluwarsa, perlu perpanjang dahulu")
	case errors.Is(err, ErrStillOverQuota):
		writeError(writer, http.StatusConflict, "still_over_quota", "kuota peer belum dinaikkan atau belum di-reset")
	case strings.Contains(err.Error(), "UNIQUE constraint failed: peers.public_key"):
		writeError(writer, http.StatusConflict, "public_key_taken", "public key sudah dipakai peer lain")
	case strings.Contains(err.Error(), "UNIQUE constraint failed: peers.interface_id, peers.name"):
		writeError(writer, http.StatusConflict, "name_taken", "nama peer sudah dipakai di interface ini")
	case strings.Contains(err.Error(), "UNIQUE constraint failed: peers.interface_id, peers.address"):
		writeError(writer, http.StatusConflict, "address_taken", "IP sudah dipakai peer lain")
	default:
		writeInternalError(writer, request, err)
	}
}

// loadPeer membaca {id} dari path. 404 bila bukan angka atau tidak ada.
func (app *App) loadPeer(writer http.ResponseWriter, request *http.Request) (sqlc.Peer, bool) {
	id, err := strconv.ParseInt(chi.URLParam(request, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(writer, http.StatusNotFound, "not_found", "peer tidak ditemukan")
		return sqlc.Peer{}, false
	}
	row, err := app.Queries.GetPeer(request.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(writer, http.StatusNotFound, "not_found", "peer tidak ditemukan")
		return sqlc.Peer{}, false
	}
	if err != nil {
		writeInternalError(writer, request, err)
		return sqlc.Peer{}, false
	}
	return row, true
}

// sealedKeypair membuat keypair client; private key dikembalikan terenkripsi, public key polos.
func (app *App) sealedKeypair() (string, []byte, error) {
	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return "", nil, err
	}
	sealed, err := app.Box.Seal([]byte(key.String()))
	return key.PublicKey().String(), sealed, err
}

// sealedPSK membuat preshared key acak terenkripsi.
func (app *App) sealedPSK() ([]byte, error) {
	key, err := wgtypes.GenerateKey()
	if err != nil {
		return nil, err
	}
	return app.Box.Seal([]byte(key.String()))
}

// nullableID mengubah *int64 opsional menjadi sql.NullInt64.
func nullableID(id *int64) sql.NullInt64 {
	if id == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *id, Valid: true}
}
