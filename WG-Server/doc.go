// Paket wgpanel adalah akar modul backend WG-WebUI: panel web untuk mengelola WireGuard
// (satu biner `wgpanel`, lihat Plan/DESIGN.md). Kode aplikasi ada di cmd/ dan internal/.
// Berkas ini dibuat di Fase 0 agar gerbang `make check` (go vet) punya paket untuk diperiksa
// sebelum paket pertama hadir di Fase 1.
package wgpanel
