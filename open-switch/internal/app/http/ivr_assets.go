// 本文件负责IVR 媒体资源接口处理。
package http

import (
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"open-switch/internal/errs"
)

const maxIVRAssetBytes = 16 << 20

type ivrAsset struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

func (d SwitchRouterDeps) ivrAssetDir() string {
	return filepath.Join(d.Config.Recordings.Dir, "prompts")
}

func (d SwitchRouterDeps) handleIVRAssets(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(d.ivrAssetDir())
	if errors.Is(err, os.ErrNotExist) {
		writeJSON(w, http.StatusOK, map[string]any{"items": []ivrAsset{}})
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	items := make([]ivrAsset, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".wav" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".wav")
		if _, err := uuid.Parse(id); err != nil {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		label, err := os.ReadFile(filepath.Join(d.ivrAssetDir(), id+".name"))
		if err != nil || len(label) == 0 {
			label = []byte(entry.Name())
		}
		items = append(items, ivrAsset{ID: id, Name: string(label), Size: info.Size()})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (d SwitchRouterDeps) handleIVRAssetUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxIVRAssetBytes+1024)
	if err := r.ParseMultipartForm(maxIVRAssetBytes); err != nil {
		writeErr(w, errs.InvalidRequest("语音文件不能超过 16 MB"))
		return
	}
	f, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, errs.InvalidRequest("请选择 WAV 文件"))
		return
	}
	defer f.Close()
	if !strings.EqualFold(filepath.Ext(header.Filename), ".wav") {
		writeErr(w, errs.InvalidRequest("只支持 WAV 文件"))
		return
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxIVRAssetBytes+1))
	if err != nil {
		writeErr(w, err)
		return
	}
	if len(raw) > maxIVRAssetBytes || !validPromptWAV(raw) {
		writeErr(w, errs.InvalidRequest("仅支持 8/16 kHz、16 位、单声道 PCM WAV（不超过 16 MB）"))
		return
	}
	if err := os.MkdirAll(d.ivrAssetDir(), 0750); err != nil {
		writeErr(w, err)
		return
	}
	id := uuid.New().String()
	if err := os.WriteFile(filepath.Join(d.ivrAssetDir(), id+".wav"), raw, 0640); err != nil {
		writeErr(w, err)
		return
	}
	name := filepath.Base(strings.ReplaceAll(header.Filename, "\\", "/"))
	if len(name) > 160 {
		name = name[:160]
	}
	_ = os.WriteFile(filepath.Join(d.ivrAssetDir(), id+".name"), []byte(name), 0640)
	writeJSON(w, http.StatusCreated, ivrAsset{ID: id, Name: name, Size: int64(len(raw))})
}

func validPromptWAV(raw []byte) bool {
	if len(raw) < 44 || string(raw[:4]) != "RIFF" || string(raw[8:12]) != "WAVE" {
		return false
	}
	fmtOK, dataOK := false, false
	for off := 12; off+8 <= len(raw); {
		size := int(binary.LittleEndian.Uint32(raw[off+4 : off+8]))
		end := off + 8 + size
		if size < 0 || end > len(raw) {
			return false
		}
		switch string(raw[off : off+4]) {
		case "fmt ":
			if size < 16 {
				return false
			}
			fmt := raw[off+8 : end]
			rate := binary.LittleEndian.Uint32(fmt[4:8])
			fmtOK = binary.LittleEndian.Uint16(fmt[:2]) == 1 && binary.LittleEndian.Uint16(fmt[2:4]) == 1 && (rate == 8000 || rate == 16000) && binary.LittleEndian.Uint16(fmt[14:16]) == 16
		case "data":
			dataOK = size > 0 && size%2 == 0
		}
		off = end + size%2
	}
	return fmtOK && dataOK
}

func (d SwitchRouterDeps) handleIVRAssetFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "assetId")
	if _, err := uuid.Parse(id); err != nil {
		writeErr(w, errs.InvalidRequest("素材 ID 无效"))
		return
	}
	path := filepath.Join(d.ivrAssetDir(), id+".wav")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		writeErr(w, errs.NotFound("语音素材不存在"))
		return
	} else if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Content-Disposition", "inline")
	http.ServeFile(w, r, path)
}
