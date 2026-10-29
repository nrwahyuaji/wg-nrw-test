// Package api: berkas ini berisi status live peer (PLAN Tugas 5.3, PRD F-5.1). Trafik per jam menyusul di Fase 8;
// di sini rx/tx = penghitung kumulatif kernel apa adanya.
package api

import (
	"net/http"
	"time"

	"wgpanel/internal/sqlc"
)

// onlineWindow: WireGuard memperbarui handshake tiap ±2 menit selama ada lalu lintas (REKEY_AFTER_TIME 120 dtk);
// 180 dtk memberi kelonggaran satu siklus sebelum peer dianggap offline.
const onlineWindow = 180 * time.Second

// now dapat diganti uji agar batas online/kedaluwarsa deterministik.
var now = time.Now

// peerStatusView adalah status live satu peer.
type peerStatusView struct {
	PeerID        int64  `json:"peer_id"`
	InterfaceID   int64  `json:"interface_id"`
	Online        bool   `json:"online"`
	LastHandshake *int64 `json:"last_handshake"` // unix detik; null = belum pernah
	Endpoint      string `json:"endpoint"`
	ReceiveBytes  int64  `json:"rx"`
	TransmitBytes int64  `json:"tx"`
}

// kernelPeer adalah hasil baca kernel per public key, dipakai status dan dashboard.
type kernelPeer struct {
	online bool
	status peerStatusView
}

// readKernelPeers membaca kernel SATU kali per interface aktif dan memetakan public key → status. Interface yang
// tidak ada di kernel atau gagal dibaca dilewati (peernya offline), supaya satu interface rusak tidak membuat
// seluruh halaman status galat. upByInterface mencatat link UP per interface.
func (app *App) readKernelPeers(interfaces []sqlc.Interface) (map[string]kernelPeer, map[int64]bool) {
	byKey := map[string]kernelPeer{}
	upByInterface := map[int64]bool{}
	cutoff := now().Add(-onlineWindow)
	for _, iface := range interfaces {
		if iface.Enabled == 0 {
			continue
		}
		state, err := app.Kernel.Inspect(iface.Name)
		if err != nil || !state.Exists {
			continue
		}
		upByInterface[iface.ID] = state.Up
		for _, status := range state.Peers {
			view := peerStatusView{
				InterfaceID: iface.ID, Endpoint: status.Endpoint,
				ReceiveBytes: status.ReceiveBytes, TransmitBytes: status.TransmitBytes,
			}
			if !status.LastHandshake.IsZero() {
				unix := status.LastHandshake.Unix()
				view.LastHandshake = &unix
			}
			online := !status.LastHandshake.IsZero() && status.LastHandshake.After(cutoff)
			view.Online = online
			byKey[status.PublicKey] = kernelPeer{online: online, status: view}
		}
	}
	return byKey, upByInterface
}

// peerStatus: GET /status (admin, operator). Satu baris per peer di DB; peer yang tidak ada di kernel (nonaktif
// atau interface mati) tampil offline tanpa galat.
func (app *App) peerStatus(writer http.ResponseWriter, request *http.Request) {
	interfaces, err := app.Queries.ListInterfaces(request.Context())
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	peers, err := app.Queries.ListPeers(request.Context())
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	byKey, _ := app.readKernelPeers(interfaces)
	views := make([]peerStatusView, 0, len(peers))
	for _, row := range peers {
		view := peerStatusView{InterfaceID: row.InterfaceID}
		if found, ok := byKey[row.PublicKey]; ok && found.status.InterfaceID == row.InterfaceID {
			view = found.status
		}
		view.PeerID = row.ID
		views = append(views, view)
	}
	writeData(writer, http.StatusOK, views)
}
