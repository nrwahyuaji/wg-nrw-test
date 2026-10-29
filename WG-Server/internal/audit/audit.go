// Package audit menulis audit_log (PLAN 10.1): apa yang berubah, oleh siapa, dari IP mana. Nilai yang bisa
// membocorkan rahasia dibuang sebelum INSERT, sehingga log aman disalin ke tiket atau surel.
package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"wgpanel/internal/sqlc"
)

// Nama aksi. Satu daftar supaya filter UI, uji, dan grep gerbang memakai string yang sama.
const (
	ActionLogin          = "auth.login"
	ActionLoginFailed    = "auth.login_failed"
	ActionLogout         = "auth.logout"
	ActionPasswordChange = "user.password_change"

	ActionInterfaceCreate = "interface.create"
	ActionInterfaceUpdate = "interface.update"

	ActionPeerCreate         = "peer.create"
	ActionPeerUpdate         = "peer.update"
	ActionPeerDelete         = "peer.delete"
	ActionPeerEnable         = "peer.enable"
	ActionPeerDisable        = "peer.disable"
	ActionPeerConfigDownload = "peer.config_download"
	ActionPeerQRDownload     = "peer.qr_download"
)

// Actor adalah pelaku perubahan. UserID 0 = sistem/anonim (mis. login gagal sebelum user dikenal).
type Actor struct {
	UserID   int64
	Username string
}

// Querier adalah bagian *sqlc.Queries yang dipakai Record. *sqlc.Queries milik transaksi Reconciler juga
// memenuhi, sehingga baris audit ikut commit atau rollback bersama perubahannya.
type Querier interface {
	InsertAudit(ctx context.Context, arg sqlc.InsertAuditParams) error
}

// Record menulis satu baris audit_log dengan waktu sekarang. detail boleh nil; isinya diredaksi dulu.
func Record(ctx context.Context, q Querier, actor Actor, ip, action, target string, detail map[string]any) error {
	params := sqlc.InsertAuditParams{
		At: time.Now().Unix(), Username: actor.Username, Ip: ip, Action: action, Target: target,
		Detail: encodeDetail(redact(detail)),
	}
	if actor.UserID != 0 {
		params.UserID = sql.NullInt64{Int64: actor.UserID, Valid: true}
	}
	return q.InsertAudit(ctx, params)
}

// secretKeyParts: kunci detail yang nilainya tidak boleh masuk audit log (tanpa peduli huruf besar/kecil).
var secretKeyParts = []string{"key", "password", "token", "secret", "code"}

// redact membuang kunci rahasia secara rekursif, termasuk di dalam map dan slice bersarang.
// Kunci yang dibuang bukan diganti penanda: lebih sedikit data lebih aman daripada nilai yang "hampir benar".
func redact(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		cleaned := make(map[string]any, len(typed))
		for key, item := range typed {
			if isSecretKey(key) {
				continue
			}
			cleaned[key] = redact(item)
		}
		return cleaned
	case []any:
		cleaned := make([]any, 0, len(typed))
		for _, item := range typed {
			cleaned = append(cleaned, redact(item))
		}
		return cleaned
	}
	return value
}

// isSecretKey melaporkan apakah nama kunci mengandung salah satu kata rahasia.
func isSecretKey(key string) bool {
	lower := strings.ToLower(key)
	for _, part := range secretKeyParts {
		if strings.Contains(lower, part) {
			return true
		}
	}
	return false
}

// encodeDetail mengubah detail menjadi JSON. Nilai yang tidak bisa di-encode (mis. channel) menjadi "{}":
// audit tidak boleh gagal hanya karena bentuk detailnya aneh.
func encodeDetail(detail any) string {
	if detail == nil {
		return "{}"
	}
	encoded, err := json.Marshal(detail)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}
