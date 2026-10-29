// Package api: berkas ini menyajikan audit log (PLAN Tugas 10.1, PRD F-7.1). Khusus admin (DESIGN 7.4).
// Daftar difilter + dipaginasi; versi CSV di-stream supaya log besar tidak masuk memori sekaligus.
package api

import (
	"context"
	"encoding/csv"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"wgpanel/internal/audit"
	"wgpanel/internal/reconcile"
	"wgpanel/internal/sqlc"
)

const (
	auditPageSize      = 50
	auditMaxPageSize   = 200
	auditCSVMaxRows    = 100000
	secondsPerDayAudit = 86400
	witaShiftSeconds   = 8 * 3600
)

// auditView adalah satu baris audit_log di respons. detail tetap string JSON: isinya sudah diredaksi
// saat ditulis, dan frontend menampilkannya apa adanya.
type auditView struct {
	ID       int64  `json:"id"`
	At       int64  `json:"at"`
	UserID   *int64 `json:"user_id"`
	Username string `json:"username"`
	IP       string `json:"ip"`
	Action   string `json:"action"`
	Target   string `json:"target"`
	Detail   string `json:"detail"`
}

// newAuditView menyalin baris audit_log.
func newAuditView(row sqlc.AuditLog) auditView {
	view := auditView{
		ID: row.ID, At: row.At, Username: row.Username, IP: row.Ip,
		Action: row.Action, Target: row.Target, Detail: row.Detail,
	}
	if row.UserID.Valid {
		view.UserID = &row.UserID.Int64
	}
	return view
}

// auditFilter adalah parameter query yang sudah divalidasi.
type auditFilter struct {
	Action   string
	Username string
	FromAt   int64
	ToAt     int64
	Page     int64
	Limit    int64
}

// parseAuditFilter membaca ?action=&user=&from=&to=&page=&per_page=. Tanggal from/to = YYYY-MM-DD (WITA).
// ok=false bila respons galat sudah dikirim.
func parseAuditFilter(writer http.ResponseWriter, request *http.Request) (auditFilter, bool) {
	query := request.URL.Query()
	filter := auditFilter{
		Action:   strings.TrimSpace(query.Get("action")),
		Username: strings.TrimSpace(query.Get("user")),
		Page:     1,
		Limit:    auditPageSize,
	}
	if value := query.Get("page"); value != "" {
		page, err := strconv.ParseInt(value, 10, 64)
		if err != nil || page < 1 {
			writeValidation(writer, []fieldError{{Field: "page", Message: "page harus angka ≥ 1"}})
			return auditFilter{}, false
		}
		filter.Page = page
	}
	if value := query.Get("per_page"); value != "" {
		limit, err := strconv.ParseInt(value, 10, 64)
		if err != nil || limit < 1 || limit > auditMaxPageSize {
			writeValidation(writer, []fieldError{{Field: "per_page", Message: "per_page harus 1–200"}})
			return auditFilter{}, false
		}
		filter.Limit = limit
	}
	filter.FromAt, filter.ToAt = 0, time.Now().Unix()
	for field, raw := range map[string]*int64{"from": &filter.FromAt, "to": &filter.ToAt} {
		value := query.Get(field)
		if value == "" {
			continue
		}
		day, err := time.ParseInLocation("2006-01-02", value, witaLocation)
		if err != nil {
			writeValidation(writer, []fieldError{{Field: field, Message: field + " harus format YYYY-MM-DD"}})
			return auditFilter{}, false
		}
		*raw = day.Unix()
		// "to" inklusif sehari penuh: geser ke detik terakhir hari WITA itu.
		if field == "to" {
			*raw += secondsPerDayAudit - 1
		}
	}
	if filter.FromAt > filter.ToAt {
		writeValidation(writer, []fieldError{{Field: "from", Message: "from tidak boleh setelah to"}})
		return auditFilter{}, false
	}
	return filter, true
}

// witaLocation = Asia/Makassar (UTC+8, tanpa DST).
var witaLocation = time.FixedZone("WITA", witaShiftSeconds)

// listAudit: GET /audit (admin). {items, page, per_page, total}.
func (app *App) listAudit(writer http.ResponseWriter, request *http.Request) {
	filter, ok := parseAuditFilter(writer, request)
	if !ok {
		return
	}
	ctx := request.Context()
	params := sqlc.ListAuditParams{
		Action: filter.Action, Username: filter.Username, FromAt: filter.FromAt, ToAt: filter.ToAt,
		PageLimit: filter.Limit, PageOffset: (filter.Page - 1) * filter.Limit,
	}
	rows, err := app.Queries.ListAudit(ctx, params)
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	total, err := app.Queries.CountAudit(ctx, sqlc.CountAuditParams{
		Action: filter.Action, Username: filter.Username, FromAt: filter.FromAt, ToAt: filter.ToAt,
	})
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	items := make([]auditView, 0, len(rows))
	for _, row := range rows {
		items = append(items, newAuditView(row))
	}
	writeData(writer, http.StatusOK, map[string]any{
		"items": items, "page": filter.Page, "per_page": filter.Limit, "total": total,
	})
}

// auditCSV: GET /audit.csv (admin). Filter sama dengan /audit, tanpa paginasi (maks 100k baris).
func (app *App) auditCSV(writer http.ResponseWriter, request *http.Request) {
	filter, ok := parseAuditFilter(writer, request)
	if !ok {
		return
	}
	filter.Limit, filter.Page = auditCSVMaxRows, 1
	rows, err := app.Queries.ListAudit(request.Context(), sqlc.ListAuditParams{
		Action: filter.Action, Username: filter.Username, FromAt: filter.FromAt, ToAt: filter.ToAt,
		PageLimit: filter.Limit, PageOffset: 0,
	})
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	writer.Header().Set("Content-Type", "text/csv; charset=utf-8")
	writer.Header().Set("Content-Disposition",
		`attachment; filename="audit-`+time.Now().In(witaLocation).Format("20060102")+`.csv"`)
	writer.Header().Set("Cache-Control", "no-store")
	csvWriter := csv.NewWriter(writer)
	_ = csvWriter.Write([]string{"id", "waktu_wita", "username", "ip", "action", "target", "detail"})
	zone := witaLocation
	for _, row := range rows {
		_ = csvWriter.Write([]string{
			strconv.FormatInt(row.ID, 10),
			time.Unix(row.At, 0).In(zone).Format("2006-01-02 15:04:05"),
			row.Username, row.Ip, row.Action, row.Target, row.Detail,
		})
	}
	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil && !errors.Is(err, http.ErrHandlerTimeout) {
		// Header sudah terkirim; galat tulis berarti klien memutus koneksi. Cukup dicatat oleh requestLog.
		return
	}
}

// auditTx menulis audit DI DALAM transaksi perubahan (tx milik Reconciler), sehingga barisnya ikut commit
// atau rollback bersama perubahan yang dicatatnya.
func (app *App) auditTx(ctx context.Context, tx reconcile.Tx, request *http.Request, action, target string, detail map[string]any) error {
	session := currentSession(request)
	return audit.Record(ctx, tx.Queries, audit.Actor{UserID: session.User.ID, Username: session.User.Username},
		ClientIP(request, app.Config.TrustedProxies), action, target, detail)
}

// recordAudit menulis audit dari handler yang tidak berada di dalam transaksi (login, logout, unduhan config).
// Galatnya hanya dicatat ke log: audit yang gagal tidak boleh membatalkan aksi yang sudah terjadi.
func (app *App) recordAudit(request *http.Request, actor audit.Actor, action, target string, detail map[string]any) {
	if err := audit.Record(request.Context(), app.Queries, actor,
		ClientIP(request, app.Config.TrustedProxies), action, target, detail); err != nil {
		slog.Error("audit gagal ditulis", "action", action, "target", target, "err", err)
	}
}
