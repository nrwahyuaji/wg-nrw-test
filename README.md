# WG-WebUI

Panel web untuk mengelola WireGuard di VPS, berupa satu biner Go (`wgpanel`) dengan SPA React ter-embed.

**Status:** Tahap I (WG-G1) dan Tahap II (WG-G2) lulus review sysadmin (9 Okt 2026, tag `tahap-II`). Tahap III: Fase 8 selesai (tag `fase-08`); berikutnya Fase 9 (masa berlaku + kuota).

| Folder | Isi |
|---|---|
| `Plan/` | dokumen perencanaan. Mulai baca dari `Plan/README.md` |
| `WG-Server/` | backend Go `wgpanel` (dibuat Fase 0–2) |
| `WG-Frontend/` | SPA React + Vite + TS (dibuat Fase 6) |
| `scripts/` | lab netns, uji handshake, uji backup |
| `docs/openapi.yaml` | kontrak API (Fase 6) |
| `AGENTS.md` | aturan coding wajib |
