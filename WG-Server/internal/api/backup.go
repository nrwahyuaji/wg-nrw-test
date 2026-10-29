// Package api: backup manual (PLAN 11.1). POST /backup {password} → bcrypt cek → stream berkas snapshot.
package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"wgpanel/internal/audit"
	"wgpanel/internal/auth"
	"wgpanel/internal/backup"
)

// backup: POST /backup {password}. Admin. Password = password admin saat ini (konfirmasi).
func (app *App) backup(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	current := currentSession(request)
	if !auth.CheckPassword(current.User.PasswordHash, body.Password) {
		writeError(writer, http.StatusForbidden, "invalid_password", "password salah")
		return
	}
	dir := app.Config.BackupDir
	if dir == "" {
		dir = "/var/lib/wgpanel/backups"
	}
	path, err := backup.Snapshot(request.Context(), app.DB, dir)
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	writer.Header().Set("Content-Type", "application/octet-stream")
	writer.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(path)+`"`)
	writer.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	writer.Header().Set("Cache-Control", "no-store")
	http.ServeContent(writer, request, info.Name(), info.ModTime(), file)
	_ = os.Remove(path)
	app.recordAudit(request, audit.Actor{UserID: current.User.ID, Username: current.User.Username},
		"backup.create", "", map[string]any{"path": path, "size": info.Size()})
}
