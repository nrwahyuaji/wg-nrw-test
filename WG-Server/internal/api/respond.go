// Package api: berkas ini menulis respons JSON berformat envelope (DESIGN 8) dan membaca body JSON.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// maxJSONBody membatasi body request agar klien tidak bisa menghabiskan memori dengan body raksasa.
const maxJSONBody = 1 << 20

// writeData mengirim {"data": value} dengan status yang diberikan. Galat encode diabaikan: header sudah
// terkirim dan penyebabnya hampir selalu klien yang memutus koneksi.
func writeData(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]any{"data": value})
}

// writeError mengirim {"error": {"code", "message"}}. code stabil berbahasa Inggris (dipakai frontend),
// message berbahasa Indonesia untuk ditampilkan.
func writeError(writer http.ResponseWriter, status int, code, message string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
}

// writeInternalError mencatat galat ke log (tanpa body request) dan mengirim 500 generik, supaya detail
// internal tidak bocor ke klien.
func writeInternalError(writer http.ResponseWriter, request *http.Request, err error) {
	slog.Error("galat internal", "method", request.Method, "path", request.URL.Path, "err", err)
	writeError(writer, http.StatusInternalServerError, "internal_error", "terjadi galat di server")
}

// decodeJSON membaca body JSON ke target. Body rusak atau terlalu besar → 400 sudah dikirim, return false.
func decodeJSON(writer http.ResponseWriter, request *http.Request, target any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(writer, request.Body, maxJSONBody)).Decode(target); err != nil {
		writeError(writer, http.StatusBadRequest, "bad_request", "body JSON tidak valid")
		return false
	}
	return true
}
