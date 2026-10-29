// Package api: berkas ini berisi grafik trafik per peer dan per interface (PLAN Tugas 8.2, PRD F-6.1).
package api

import (
	"context"
	"net/http"

	"wgpanel/internal/sqlc"
)

const (
	secondsPerHour = 3600
	secondsPerDay  = 86400
	// witaOffset: WITA = UTC+8 tanpa DST, jadi batas hari WITA = kelipatan 86400 setelah digeser 8 jam.
	witaOffset = 8 * secondsPerHour
)

// trafficPoint adalah satu titik grafik: at = awal jam (24h) atau awal hari WITA (7d/30d), unix detik.
type trafficPoint struct {
	At      int64 `json:"at"`
	RxBytes int64 `json:"rx"`
	TxBytes int64 `json:"tx"`
}

// trafficRange memetakan ?range= ke jumlah titik dan lebar ember (detik). false = range tidak dikenal.
func trafficRange(value string) (points int64, bucket int64, ok bool) {
	switch value {
	case "", "24h":
		return 24, secondsPerHour, true
	case "7d":
		return 7, secondsPerDay, true
	case "30d":
		return 30, secondsPerDay, true
	}
	return 0, 0, false
}

// witaDayStart mengembalikan awal hari WITA (unix) yang memuat moment.
func witaDayStart(moment int64) int64 {
	return (moment+witaOffset)/secondsPerDay*secondsPerDay - witaOffset
}

// loadTraffic membaca titik grafik untuk satu peer (peerID) atau satu interface (interfaceID); id lain = 0.
// Ember tanpa data diisi 0 di Go supaya grafik tidak bolong dan jumlah titik selalu tetap (24/7/30).
func (app *App) loadTraffic(ctx context.Context, rangeValue string, peerID, interfaceID int64) ([]trafficPoint, bool, error) {
	points, bucket, ok := trafficRange(rangeValue)
	if !ok {
		return nil, false, nil
	}
	moment := now().Unix()
	var first int64
	sums := map[int64]trafficPoint{}
	if bucket == secondsPerHour {
		first = moment - moment%secondsPerHour - (points-1)*secondsPerHour
		rows, err := app.Queries.TrafficByHour(ctx, sqlc.TrafficByHourParams{PeerID: peerID, InterfaceID: interfaceID, Since: first})
		if err != nil {
			return nil, true, err
		}
		for _, row := range rows {
			sums[row.Bucket] = trafficPoint{At: row.Bucket, RxBytes: row.RxBytes, TxBytes: row.TxBytes}
		}
	} else {
		first = witaDayStart(moment) - (points-1)*secondsPerDay
		rows, err := app.Queries.TrafficByDay(ctx, sqlc.TrafficByDayParams{PeerID: peerID, InterfaceID: interfaceID, Since: first})
		if err != nil {
			return nil, true, err
		}
		for _, row := range rows {
			// SQL mengembalikan indeks hari WITA; dikembalikan ke unix awal hari WITA agar sama dengan ember Go.
			start := row.Bucket*secondsPerDay - witaOffset
			sums[start] = trafficPoint{At: start, RxBytes: row.RxBytes, TxBytes: row.TxBytes}
		}
	}
	result := make([]trafficPoint, 0, points)
	for index := range points {
		at := first + index*bucket
		point := sums[at]
		point.At = at
		result = append(result, point)
	}
	return result, true, nil
}

// writeTraffic mengirim titik grafik, 422 untuk range tidak dikenal, atau 500.
func writeTraffic(writer http.ResponseWriter, request *http.Request, points []trafficPoint, ok bool, err error) {
	switch {
	case err != nil:
		writeInternalError(writer, request, err)
	case !ok:
		writeValidation(writer, []fieldError{{Field: "range", Message: "range harus 24h, 7d, atau 30d"}})
	default:
		writeData(writer, http.StatusOK, points)
	}
}

// peerTraffic: GET /peers/{id}/traffic?range=24h|7d|30d (admin, operator). 404 peer tidak ada.
func (app *App) peerTraffic(writer http.ResponseWriter, request *http.Request) {
	row, found := app.loadPeer(writer, request)
	if !found {
		return
	}
	points, ok, err := app.loadTraffic(request.Context(), request.URL.Query().Get("range"), row.ID, 0)
	writeTraffic(writer, request, points, ok, err)
}

// interfaceTraffic: GET /interfaces/{id}/traffic?range= (admin, operator). Jumlah semua peer interface itu.
func (app *App) interfaceTraffic(writer http.ResponseWriter, request *http.Request) {
	row, found := app.loadInterface(writer, request)
	if !found {
		return
	}
	points, ok, err := app.loadTraffic(request.Context(), request.URL.Query().Get("range"), 0, row.ID)
	writeTraffic(writer, request, points, ok, err)
}
