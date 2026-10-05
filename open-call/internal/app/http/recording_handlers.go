// 本文件负责录音查询、下载与清理接口。
package http

import (
	"io"
	"net/http"
	"open-call/internal/errs"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

func (d RouterDeps) handleRecordingList(w http.ResponseWriter, r *http.Request) {
	page, size := pageParams(r)
	out, err := d.Recordings.List(r.Context(), page, size, r.URL.Query().Get("call_id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d RouterDeps) handleRecordingDownload(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(r)
	if !ok {
		writeErr(w, errs.Unauthorized("未认证或令牌失效"))
		return
	}
	row, err := d.Recordings.Get(r.Context(), chi.URLParam(r, "recordingId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format != "" {
		if (format != "webm" && format != "mp4") || row.MediaType != "video_composite" ||
			(filepath.Ext(row.FilePath) != ".webm" && filepath.Ext(row.FilePath) != ".mp4") {
			writeErr(w, errs.InvalidRequest("录像格式仅支持 webm 或 mp4"))
			return
		}
	}
	d.writeAudit(r.Context(), p.UserID, "recording_download", row.ID, map[string]string{"call_id": row.CallID, "format": format})
	f, err := d.Recordings.Open(r.Context(), row, format)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = f.Close() }()
	ext := filepath.Ext(row.FilePath)
	if format != "" {
		ext = "." + format
	}
	name := row.ID + ext
	if name == row.ID {
		name = row.ID + ".ogg"
	}
	ctype := "application/octet-stream"
	switch strings.ToLower(ext) {
	case ".ogg":
		ctype = "audio/ogg"
	case ".webm":
		ctype = "video/webm"
	case ".mp4":
		ctype = "video/mp4"
	case ".ivf":
		ctype = "video/x-ivf"
	case ".wav":
		ctype = "audio/wav"
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Disposition", "attachment; filename="+name)
	_, _ = io.Copy(w, f)
}

func (d RouterDeps) handlePurgeRecordings(w http.ResponseWriter, r *http.Request) {
	n, err := d.Recordings.PurgeExpired(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	if p, ok := principal(r); ok {
		d.writeAudit(r.Context(), p.UserID, "recording_purge", "", map[string]int{"deleted": n})
	}
	writeJSON(w, http.StatusOK, map[string]int{"deleted_count": n})
}
