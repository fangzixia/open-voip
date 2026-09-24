package http

import (
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"net/http"
	"open-switch/internal/errs"
	"os"
)

func (d SwitchRouterDeps) handleRecordingFile(w http.ResponseWriter, r *http.Request) {
	callID, id := chi.URLParam(r, "callId"), chi.URLParam(r, "recordingId")
	if _, err := uuid.Parse(callID); err != nil {
		writeErr(w, errs.InvalidRequest("call_id 无效"))
		return
	}
	if _, err := uuid.Parse(id); err != nil {
		writeErr(w, errs.InvalidRequest("recording_id 无效"))
		return
	}
	ext := r.URL.Query().Get("ext")
	switch ext {
	case ".wav", ".ogg", ".webm", ".ivf":
	default:
		writeErr(w, errs.InvalidRequest("录音格式无效"))
		return
	}
	root, err := os.OpenRoot(d.Config.Recordings.Dir)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer root.Close()
	name := callID + "-" + id + ext
	if r.Method == http.MethodDelete {
		for _, suffix := range []string{".wav", ".ogg", ".webm", ".ivf"} {
			if err := root.Remove(callID + "-" + id + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
				writeErr(w, err)
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	file, err := root.Open(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeErr(w, errs.NotFound("录音文件不存在"))
		} else {
			writeErr(w, err)
		}
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+id+ext+`"`)
	http.ServeContent(w, r, name, info.ModTime(), file)
}
