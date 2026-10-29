// Package api: berkas ini berisi handler interface WireGuard server (PLAN Tugas 4.2, PRD F-2.1–F-2.4).
// Semua penulisan lewat Reconciler.WithApply supaya DB dan kernel berubah bersama atau tidak sama sekali.
package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"wgpanel/internal/audit"
	"wgpanel/internal/ipam"
	"wgpanel/internal/reconcile"
	"wgpanel/internal/sqlc"
)

// interfaceNamePattern: nama kernel ≤ 15 karakter, huruf kecil di depan, tanpa karakter yang bisa
// bermakna di shell/berkas (nama dipakai juga untuk /etc/wireguard/<nama>.conf).
var interfaceNamePattern = regexp.MustCompile(`^[a-z][a-z0-9]{0,14}$`)

// Batas MTU: 1280 = minimum IPv6 (lebih kecil merusak tunnel IPv6), 9000 = jumbo frame. 0 = bawaan 1420.
const (
	minMTU = 1280
	maxMTU = 9000
	// maxKeepalive = batas protokol WireGuard (uint16 detik).
	maxKeepalive = 65535
)

// interfaceRequest adalah body POST/PUT /interfaces. Pointer pada enabled membedakan "tidak dikirim"
// (bawaan aktif) dari false.
type interfaceRequest struct {
	Name             string   `json:"name"`
	Address          string   `json:"address"`
	ListenPort       int      `json:"listen_port"`
	MTU              int      `json:"mtu"`
	EndpointHost     string   `json:"endpoint_host"`
	ClientDNS        []string `json:"client_dns"`
	ClientAllowedIPs []string `json:"client_allowed_ips"`
	ClientKeepalive  *int     `json:"client_keepalive"`
	Enabled          *bool    `json:"enabled"`
}

// interfaceInput adalah hasil validasi interfaceRequest, siap ditulis ke DB.
type interfaceInput struct {
	Name, Address, EndpointHost, ClientDNS, ClientAllowedIPs string
	ListenPort, MTU, ClientKeepalive, Enabled                int64
}

// fieldError adalah satu galat validasi per field; dikirim dalam error.fields agar frontend bisa menandai input.
type fieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// interfaceView adalah bentuk interface di respons API. TIDAK punya field private key (N-1, DESIGN 8).
type interfaceView struct {
	ID               int64    `json:"id"`
	Name             string   `json:"name"`
	Address          string   `json:"address"`
	ListenPort       int64    `json:"listen_port"`
	MTU              int64    `json:"mtu"`
	PublicKey        string   `json:"public_key"`
	EndpointHost     string   `json:"endpoint_host"`
	ClientDNS        []string `json:"client_dns"`
	ClientAllowedIPs []string `json:"client_allowed_ips"`
	ClientKeepalive  int64    `json:"client_keepalive"`
	Enabled          bool     `json:"enabled"`
	CreatedAt        int64    `json:"created_at"`
	UpdatedAt        int64    `json:"updated_at"`
}

// newInterfaceView menyalin field publik baris interface.
func newInterfaceView(row sqlc.Interface) interfaceView {
	return interfaceView{
		ID: row.ID, Name: row.Name, Address: row.Address, ListenPort: row.ListenPort, MTU: row.Mtu,
		PublicKey: row.PublicKey, EndpointHost: row.EndpointHost, ClientDNS: splitCSV(row.ClientDns),
		ClientAllowedIPs: splitCSV(row.ClientAllowedIps), ClientKeepalive: row.ClientKeepalive,
		Enabled: row.Enabled == 1, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

// listInterfaces: GET /interfaces (admin, operator). Daftar tanpa field rahasia.
func (app *App) listInterfaces(writer http.ResponseWriter, request *http.Request) {
	rows, err := app.Queries.ListInterfaces(request.Context())
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	views := make([]interfaceView, 0, len(rows))
	for _, row := range rows {
		views = append(views, newInterfaceView(row))
	}
	writeData(writer, http.StatusOK, views)
}

// getInterface: GET /interfaces/{id} (admin, operator). 404 bila tidak ada.
func (app *App) getInterface(writer http.ResponseWriter, request *http.Request) {
	row, ok := app.loadInterface(writer, request)
	if !ok {
		return
	}
	writeData(writer, http.StatusOK, newInterfaceView(row))
}

// createInterface: POST /interfaces (admin). Keypair dibuat server; private key disimpan terenkripsi.
// 201 + interfaceView; 422 validasi; 409 nama/port sudah dipakai; kernel menolak → galat, DB tidak berubah.
func (app *App) createInterface(writer http.ResponseWriter, request *http.Request) {
	var body interfaceRequest
	if !decodeJSON(writer, request, &body) {
		return
	}
	input, fields := buildInterfaceInput(body)
	if len(fields) > 0 {
		writeValidation(writer, fields)
		return
	}
	privateKey, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	sealed, err := app.Box.Seal([]byte(privateKey.String()))
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	var created sqlc.Interface
	now := time.Now().Unix()
	err = app.Reconciler.WithApply(request.Context(), 0, func(tx reconcile.Tx) (int64, error) {
		row, err := tx.Queries.CreateInterface(request.Context(), sqlc.CreateInterfaceParams{
			Name: input.Name, Address: input.Address, ListenPort: input.ListenPort, Mtu: input.MTU,
			PrivateKey: sealed, PublicKey: privateKey.PublicKey().String(), EndpointHost: input.EndpointHost,
			ClientDns: input.ClientDNS, ClientAllowedIps: input.ClientAllowedIPs, ClientKeepalive: input.ClientKeepalive,
			Enabled: input.Enabled, CreatedAt: now, UpdatedAt: now,
		})
		created = row
		if err != nil {
			return 0, err
		}
		return row.ID, app.auditTx(request.Context(), tx, request, audit.ActionInterfaceCreate, target("interface", row.ID),
			map[string]any{"name": row.Name, "address": row.Address, "listen_port": row.ListenPort})
	})
	if err != nil {
		app.writeWriteError(writer, request, err)
		return
	}
	writeData(writer, http.StatusCreated, newInterfaceView(created))
}

// updateInterface: PUT /interfaces/{id} (admin). Field sama dengan POST. Ganti address saat interface masih
// punya peer → 422 subnet_locked (temuan spike DESIGN 10: IP peer lama akan jatuh di luar subnet).
// Nama tidak bisa diganti (422 name_immutable): link kernel lama akan tertinggal tanpa pemilik di DB, dan
// salinan /etc/wireguard/<nama>.conf ikut pindah. Ganti nama = buat interface baru (Fase 15 multi-interface).
func (app *App) updateInterface(writer http.ResponseWriter, request *http.Request) {
	current, ok := app.loadInterface(writer, request)
	if !ok {
		return
	}
	var body interfaceRequest
	if !decodeJSON(writer, request, &body) {
		return
	}
	input, fields := buildInterfaceInput(body)
	if len(fields) > 0 {
		writeValidation(writer, fields)
		return
	}
	if input.Name != current.Name {
		writeError(writer, http.StatusUnprocessableEntity, "name_immutable", "nama interface tidak bisa diganti")
		return
	}
	var updated sqlc.Interface
	err := app.Reconciler.WithApply(request.Context(), current.ID, func(tx reconcile.Tx) (int64, error) {
		// Dicek di dalam transaksi + mutex Reconciler, sehingga peer yang ditambah bersamaan tidak lolos.
		if input.Address != current.Address {
			count, err := tx.Queries.CountPeersByInterface(request.Context(), current.ID)
			if err != nil {
				return 0, err
			}
			if count > 0 {
				return 0, errSubnetLocked
			}
		}
		row, err := tx.Queries.UpdateInterface(request.Context(), sqlc.UpdateInterfaceParams{
			Name: input.Name, Address: input.Address, ListenPort: input.ListenPort, Mtu: input.MTU,
			EndpointHost: input.EndpointHost, ClientDns: input.ClientDNS, ClientAllowedIps: input.ClientAllowedIPs,
			ClientKeepalive: input.ClientKeepalive, Enabled: input.Enabled, UpdatedAt: time.Now().Unix(), ID: current.ID,
		})
		updated = row
		if err != nil {
			return 0, err
		}
		changes := map[string]any{"name": row.Name, "address": row.Address, "listen_port": row.ListenPort}
		if current.Address != row.Address {
			changes["address_before"] = current.Address
		}
		if current.Enabled != row.Enabled {
			changes["enabled"] = row.Enabled == 1
		}
		return 0, app.auditTx(request.Context(), tx, request, audit.ActionInterfaceUpdate, target("interface", row.ID), changes)
	})
	if err != nil {
		app.writeWriteError(writer, request, err)
		return
	}
	writeData(writer, http.StatusOK, newInterfaceView(updated))
}

// errSubnetLocked menandai penolakan ganti subnet; dipetakan ke 422 oleh writeWriteError.
var errSubnetLocked = errors.New("subnet terkunci")

// writeWriteError memetakan galat penulisan interface ke respons: subnet terkunci 422, UNIQUE 409, selain
// itu 500 (termasuk kernel menolak; detailnya hanya di log).
func (app *App) writeWriteError(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, errSubnetLocked):
		writeError(writer, http.StatusUnprocessableEntity, "subnet_locked",
			"subnet tidak bisa diganti selama interface masih punya peer")
	case strings.Contains(err.Error(), "UNIQUE constraint failed: interfaces.name"):
		writeError(writer, http.StatusConflict, "name_taken", "nama interface sudah dipakai")
	case strings.Contains(err.Error(), "UNIQUE constraint failed: interfaces.listen_port"):
		writeError(writer, http.StatusConflict, "port_taken", "port sudah dipakai interface lain")
	default:
		writeInternalError(writer, request, err)
	}
}

// loadInterface membaca {id} dari path. 404 bila id bukan angka atau tidak ada; return false bila respons
// sudah dikirim.
func (app *App) loadInterface(writer http.ResponseWriter, request *http.Request) (sqlc.Interface, bool) {
	id, err := strconv.ParseInt(chi.URLParam(request, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(writer, http.StatusNotFound, "not_found", "interface tidak ditemukan")
		return sqlc.Interface{}, false
	}
	row, err := app.Queries.GetInterface(request.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(writer, http.StatusNotFound, "not_found", "interface tidak ditemukan")
		return sqlc.Interface{}, false
	}
	if err != nil {
		writeInternalError(writer, request, err)
		return sqlc.Interface{}, false
	}
	return row, true
}

// buildInterfaceInput memvalidasi body POST/PUT di satu tempat (PLAN 4.2) dan mengumpulkan SEMUA galat field
// sekaligus, supaya admin memperbaiki form dalam satu kali kirim.
func buildInterfaceInput(body interfaceRequest) (interfaceInput, []fieldError) {
	var fields []fieldError
	add := func(field, message string) { fields = append(fields, fieldError{Field: field, Message: message}) }
	input := interfaceInput{Name: strings.TrimSpace(body.Name), MTU: int64(body.MTU), Enabled: 1, ClientKeepalive: 25}

	if !interfaceNamePattern.MatchString(input.Name) {
		add("name", "nama harus huruf kecil/angka, diawali huruf, maksimal 15 karakter (mis. wg0)")
	}
	if prefix, err := ipam.ParseServerPrefix(strings.TrimSpace(body.Address)); err != nil {
		add("address", "alamat harus CIDR IPv4 server, mis. 10.8.0.1/24 (minimal /30, bukan alamat network/broadcast)")
	} else {
		input.Address = prefix.String()
	}
	if body.ListenPort < 1 || body.ListenPort > 65535 {
		add("listen_port", "port harus 1–65535")
	}
	input.ListenPort = int64(body.ListenPort)
	if body.MTU != 0 && (body.MTU < minMTU || body.MTU > maxMTU) {
		add("mtu", "MTU harus 0 (bawaan) atau 1280–9000")
	}
	input.EndpointHost = strings.TrimSpace(body.EndpointHost)
	if !validEndpointHost(input.EndpointHost) {
		add("endpoint_host", "endpoint wajib: nama host atau IP publik, tanpa skema dan tanpa port")
	}
	dns := make([]string, 0, len(body.ClientDNS))
	for _, item := range body.ClientDNS {
		addr, err := netip.ParseAddr(strings.TrimSpace(item))
		if err != nil {
			add("client_dns", "DNS harus alamat IP: "+item)
			continue
		}
		dns = append(dns, addr.String())
	}
	input.ClientDNS = strings.Join(dns, ",")
	allowed := body.ClientAllowedIPs
	if len(allowed) == 0 {
		allowed = []string{"0.0.0.0/0"} // bawaan skema: full tunnel
	}
	normalized := make([]string, 0, len(allowed))
	for _, item := range allowed {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(item))
		if err != nil {
			add("client_allowed_ips", "AllowedIPs harus CIDR: "+item)
			continue
		}
		normalized = append(normalized, prefix.Masked().String())
	}
	input.ClientAllowedIPs = strings.Join(normalized, ",")
	if body.ClientKeepalive != nil {
		if *body.ClientKeepalive < 0 || *body.ClientKeepalive > maxKeepalive {
			add("client_keepalive", "keepalive harus 0–65535 detik")
		}
		input.ClientKeepalive = int64(*body.ClientKeepalive)
	}
	if body.Enabled != nil && !*body.Enabled {
		input.Enabled = 0
	}
	return input, fields
}

// validEndpointHost menerima IP atau nama host DNS. Ditolak: kosong, skema (http://), port, spasi, dan
// path, karena nilainya ditulis apa adanya ke baris `Endpoint = <host>:<port>` config client.
func validEndpointHost(host string) bool {
	if host == "" || len(host) > 253 {
		return false
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return true
	}
	if strings.ContainsAny(host, ":/ @") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, char := range label {
			if !isHostnameChar(char) {
				return false
			}
		}
	}
	return true
}

// isHostnameChar: huruf ASCII, angka, atau tanda hubung (RFC 1123).
func isHostnameChar(char rune) bool {
	return char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-'
}

// writeValidation mengirim 422 validation_failed beserta daftar galat per field.
func writeValidation(writer http.ResponseWriter, fields []fieldError) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusUnprocessableEntity)
	_ = json.NewEncoder(writer).Encode(map[string]any{"error": map[string]any{
		"code": "validation_failed", "message": "periksa kembali isian", "fields": fields,
	}})
}

// splitCSV memecah kolom dipisah koma menjadi slice; kolom kosong → slice kosong (bukan null di JSON).
func splitCSV(raw string) []string {
	result := []string{}
	for _, item := range strings.Split(raw, ",") {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}
