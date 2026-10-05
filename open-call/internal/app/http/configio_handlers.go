// 本文件负责配置导入导出接口。
package http

import (
	"net/http"
	"open-call/internal/layers/biz/configio"
)

func (d RouterDeps) handleConfigExport(w http.ResponseWriter, r *http.Request) {
	out, err := d.ConfigIO.Export(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d RouterDeps) handleConfigImport(w http.ResponseWriter, r *http.Request) {
	var bundle configio.Bundle
	if err := decodeJSON(r, &bundle); err != nil {
		writeErr(w, err)
		return
	}
	options := configio.ImportOptions{DryRun: r.URL.Query().Get("dry_run") == "true", Mode: r.URL.Query().Get("mode")}
	report, err := d.ConfigIO.ImportWithOptions(r.Context(), bundle, options)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !options.DryRun {
		if p, ok := principal(r); ok && d.Audit != nil {
			if err := d.Audit.Write(r.Context(), p.UserID, "config_import", "", map[string]string{"outcome": "success"}); err != nil {
				writeErr(w, err)
				return
			}
		}
	}
	writeJSON(w, http.StatusOK, report)
}
