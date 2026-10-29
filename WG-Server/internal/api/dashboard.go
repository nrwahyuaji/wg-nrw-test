// Package api: berkas ini berisi ringkasan dashboard (PLAN Tugas 5.3 + 8.2, PRD F-5.2, F-6.1).
package api

import (
	"database/sql"
	"net/http"
	"time"

	"wgpanel/internal/sqlc"
)

// expiringWindow: peer yang kedaluwarsa dalam 7 hari tampil di dashboard.
const expiringWindow = 7 * 24 * time.Hour

// dashboardInterface adalah ringkasan satu interface di dashboard.
type dashboardInterface struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Enabled     bool   `json:"enabled"`
	Up          bool   `json:"up"`
	Peers       int64  `json:"peers"`
	PeersOnline int64  `json:"peers_online"`
}

// expiringPeer adalah peer yang segera kedaluwarsa.
type expiringPeer struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	InterfaceID int64  `json:"interface_id"`
	ExpiresAt   int64  `json:"expires_at"`
}

// topPeersLimit: jumlah peer terboros yang ditampilkan dashboard.
const topPeersLimit = 5

// topPeer adalah pemakaian satu peer sejak awal hari WITA.
type topPeer struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	InterfaceID int64  `json:"interface_id"`
	RxBytes     int64  `json:"rx"`
	TxBytes     int64  `json:"tx"`
}

// dashboardView adalah respons GET /dashboard. Trafik "hari ini" = sejak 00.00 WITA.
type dashboardView struct {
	PeersTotal   int64                `json:"peers_total"`
	PeersEnabled int64                `json:"peers_enabled"`
	PeersOnline  int64                `json:"peers_online"`
	Interfaces   []dashboardInterface `json:"interfaces"`
	ExpiringSoon []expiringPeer       `json:"expiring_soon"`
	TodayRxBytes int64                `json:"today_rx"`
	TodayTxBytes int64                `json:"today_tx"`
	TopPeers     []topPeer            `json:"top_peers_today"`
}

// dashboard: GET /dashboard (admin, operator). Hitungan diambil dengan GROUP BY (tanpa N+1); online dari satu
// pembacaan kernel per interface.
func (app *App) dashboard(writer http.ResponseWriter, request *http.Request) {
	ctx := request.Context()
	interfaces, err := app.Queries.ListInterfaces(ctx)
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	counts, err := app.Queries.CountPeersByInterfaceGrouped(ctx)
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	expiring, err := app.Queries.ListExpiringPeers(ctx, sql.NullInt64{Int64: now().Add(expiringWindow).Unix(), Valid: true})
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	// Satu query GROUP BY peer: jumlahnya = total hari ini, 5 baris teratas = top 5 (sudah diurutkan SQL).
	usage, err := app.Queries.TrafficByPeerSince(ctx, witaDayStart(now().Unix()))
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	byKey, upByInterface := app.readKernelPeers(interfaces)
	countByInterface := make(map[int64]sqlc.CountPeersByInterfaceGroupedRow, len(counts))
	view := dashboardView{Interfaces: []dashboardInterface{}, ExpiringSoon: []expiringPeer{}, TopPeers: []topPeer{}}
	for _, row := range usage {
		view.TodayRxBytes += row.RxBytes
		view.TodayTxBytes += row.TxBytes
		if len(view.TopPeers) < topPeersLimit {
			view.TopPeers = append(view.TopPeers, topPeer{
				ID: row.ID, Name: row.Name, InterfaceID: row.InterfaceID, RxBytes: row.RxBytes, TxBytes: row.TxBytes,
			})
		}
	}
	for _, row := range counts {
		countByInterface[row.InterfaceID] = row
		view.PeersTotal += row.Total
		view.PeersEnabled += row.Enabled
	}
	onlineByInterface := map[int64]int64{}
	for _, found := range byKey {
		if found.online {
			onlineByInterface[found.status.InterfaceID]++
			view.PeersOnline++
		}
	}
	for _, iface := range interfaces {
		view.Interfaces = append(view.Interfaces, dashboardInterface{
			ID: iface.ID, Name: iface.Name, Enabled: iface.Enabled == 1, Up: upByInterface[iface.ID],
			Peers: countByInterface[iface.ID].Total, PeersOnline: onlineByInterface[iface.ID],
		})
	}
	for _, row := range expiring {
		view.ExpiringSoon = append(view.ExpiringSoon, expiringPeer{
			ID: row.ID, Name: row.Name, InterfaceID: row.InterfaceID, ExpiresAt: row.ExpiresAt.Int64,
		})
	}
	writeData(writer, http.StatusOK, view)
}
