// Package api: tag peer (PLAN 12.1). Admin.
package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"wgpanel/internal/reconcile"
	"wgpanel/internal/sqlc"
)

// listTags: GET /tags (admin). Daftar semua tag.
func (app *App) listTags(writer http.ResponseWriter, request *http.Request) {
	rows, err := app.Queries.ListTags(request.Context())
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	type view struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	views := make([]view, 0, len(rows))
	for _, row := range rows {
		views = append(views, view{ID: row.ID, Name: row.Name})
	}
	writeData(writer, http.StatusOK, views)
}

// createTag: POST /tags {name}. Admin. Upsert berdasarkan nama (case-insensitive).
func (app *App) createTag(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeValidation(writer, []fieldError{{Field: "name", Message: "nama tag wajib diisi"}})
		return
	}
	if existing, err := app.Queries.GetTagByName(request.Context(), name); err == nil {
		writeData(writer, http.StatusOK, map[string]any{"id": existing.ID, "name": existing.Name})
		return
	}
	row, err := app.Queries.CreateTag(request.Context(), name)
	if err != nil {
		if existing, err2 := app.Queries.GetTagByName(request.Context(), name); err2 == nil {
			writeData(writer, http.StatusOK, map[string]any{"id": existing.ID, "name": existing.Name})
			return
		}
		writeInternalError(writer, request, err)
		return
	}
	writeData(writer, http.StatusCreated, map[string]any{"id": row.ID, "name": row.Name})
}

// deleteTag: DELETE /tags/{id}. Admin. 204.
func (app *App) deleteTag(writer http.ResponseWriter, request *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(request, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(writer, http.StatusNotFound, "not_found", "tag tidak ditemukan")
		return
	}
	if err := app.Queries.DeleteTag(request.Context(), id); err != nil {
		writeInternalError(writer, request, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

// peerTags: GET /peers/{id}/tags (admin, operator). Daftar nama tag.
func (app *App) peerTags(writer http.ResponseWriter, request *http.Request) {
	current, ok := app.loadPeer(writer, request)
	if !ok {
		return
	}
	names, err := app.Queries.ListPeerTags(request.Context(), current.ID)
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	writeData(writer, http.StatusOK, names)
}

// setPeerTags: PUT /peers/{id}/tags {tags: []string}. Admin. Clear + reinsert di transaksi.
func (app *App) setPeerTags(writer http.ResponseWriter, request *http.Request) {
	current, ok := app.loadPeer(writer, request)
	if !ok {
		return
	}
	var body struct {
		Tags []string `json:"tags"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	ctx := request.Context()
	err := app.Reconciler.WithApply(ctx, current.InterfaceID, func(tx reconcile.Tx) (int64, error) {
		if err := tx.Queries.DeletePeerTags(ctx, current.ID); err != nil {
			return 0, err
		}
		for _, name := range body.Tags {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			tag, err := tx.Queries.GetTagByName(ctx, name)
			if errors.Is(err, sql.ErrNoRows) {
				tag, err = tx.Queries.CreateTag(ctx, name)
			}
			if err != nil {
				return 0, err
			}
			if err := tx.Queries.AddPeerTag(ctx, sqlc.AddPeerTagParams{PeerID: current.ID, TagID: tag.ID}); err != nil {
				return 0, err
			}
		}
		return 0, nil
	})
	if err != nil {
		writeInternalError(writer, request, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}
