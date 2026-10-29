// Package api: berkas ini berisi GET /interfaces/{id}/health (PLAN Tugas 4.3, PRD F-5.3, F-5.4): kecocokan
// kernel vs DB dan peringatan forwarding. Panel tidak mengubah sysctl/firewall (K7); ia hanya memberi tahu.
package api

import (
	"net/http"
	"net/netip"
	"os"
	"strings"

	"wgpanel/internal/sqlc"
	"wgpanel/internal/wg"
)

// ipForwardPath dibaca untuk peringatan full-tunnel. Variabel (bukan konstanta) agar uji bisa menggantinya.
var ipForwardPath = "/proc/sys/net/ipv4/ip_forward"

// KernelInspector adalah pembacaan kernel yang dipakai handler health. Produksi = *wg.Engine; uji = palsu.
type KernelInspector interface {
	Inspect(name string) (wg.InterfaceState, error)
}

// healthView adalah respons health. ip_forward null bila berkasnya tidak terbaca (mis. sandbox ketat).
type healthView struct {
	InKernel         bool     `json:"in_kernel"`
	Up               bool     `json:"up"`
	ListenPortKernel int      `json:"listen_port_kernel"`
	ListenPortDB     int64    `json:"listen_port_db"`
	PeersKernel      int      `json:"peers_kernel"`
	PeersDB          int64    `json:"peers_db"`
	IPForward        *bool    `json:"ip_forward"`
	Warnings         []string `json:"warnings"`
}

// Kode peringatan stabil (dipakai frontend untuk teks dan tautan bantuan).
const (
	warnNotInKernel  = "not_in_kernel"  // interface aktif di DB tetapi tidak ada di kernel
	warnDown         = "link_down"      // link ada tetapi tidak UP
	warnPortMismatch = "port_mismatch"  // port kernel ≠ DB
	warnPeerMismatch = "peer_mismatch"  // jumlah peer kernel ≠ peer aktif DB
	warnIPForwardOff = "ip_forward_off" // ada peer full-tunnel, tetapi host tidak meneruskan paket
	warnDisabled     = "interface_disabled"
)

// interfaceHealth: GET /interfaces/{id}/health (admin, operator).
func (app *App) interfaceHealth(writer http.ResponseWriter, request *http.Request) {
	iface, ok := app.loadInterface(writer, request)
	if !ok {
		return
	}
	peers, err := app.Queries.ListActivePeersByInterface(request.Context(), iface.ID)
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	state, err := app.Kernel.Inspect(iface.Name)
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	view := healthView{
		InKernel: state.Exists, Up: state.Up, ListenPortKernel: state.ListenPort, ListenPortDB: iface.ListenPort,
		PeersKernel: len(state.Peers), PeersDB: int64(len(peers)), IPForward: readIPForward(), Warnings: []string{},
	}
	if iface.Enabled == 0 {
		view.PeersDB = 0 // interface nonaktif sengaja tidak ada di kernel
		view.Warnings = append(view.Warnings, warnDisabled)
		writeData(writer, http.StatusOK, view)
		return
	}
	switch {
	case !state.Exists:
		view.Warnings = append(view.Warnings, warnNotInKernel)
	default:
		if !state.Up {
			view.Warnings = append(view.Warnings, warnDown)
		}
		if int64(state.ListenPort) != iface.ListenPort {
			view.Warnings = append(view.Warnings, warnPortMismatch)
		}
		if int64(len(state.Peers)) != view.PeersDB {
			view.Warnings = append(view.Warnings, warnPeerMismatch)
		}
	}
	// Peer full-tunnel (AllowedIPs client memuat 0.0.0.0/0) hanya bisa ke internet bila host meneruskan paket.
	if view.IPForward != nil && !*view.IPForward && hasFullTunnelPeer(iface.ClientAllowedIps, peers) {
		view.Warnings = append(view.Warnings, warnIPForwardOff)
	}
	writeData(writer, http.StatusOK, view)
}

// hasFullTunnelPeer: ada peer aktif yang rute client-nya mencakup seluruh IPv4. Kolom peer kosong = ikut interface.
func hasFullTunnelPeer(interfaceAllowed string, peers []sqlc.Peer) bool {
	for _, peer := range peers {
		allowed := peer.ClientAllowedIps
		if strings.TrimSpace(allowed) == "" {
			allowed = interfaceAllowed
		}
		for _, item := range splitCSV(allowed) {
			if prefix, err := netip.ParsePrefix(item); err == nil && prefix.Bits() == 0 && prefix.Addr().Is4() {
				return true
			}
		}
	}
	return false
}

// readIPForward membaca sysctl ip_forward; nil bila tidak terbaca (bukan galat request).
func readIPForward() *bool {
	raw, err := os.ReadFile(ipForwardPath)
	if err != nil {
		return nil
	}
	enabled := strings.TrimSpace(string(raw)) == "1"
	return &enabled
}
