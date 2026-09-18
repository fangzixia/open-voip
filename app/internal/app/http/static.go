package http

import (
	"net/http"
	"os"
	"path/filepath"
)

// mountStaticApps 将 static 目录下的 agent/guest/admin 挂载到对应 URL 前缀。
func mountStaticApps(r interface {
	Get(pattern string, h http.HandlerFunc)
	Handle(pattern string, h http.Handler)
}, staticDir string) {
	if staticDir == "" {
		return
	}

	apps := []struct {
		prefix string
		dir    string
	}{
		{"/agent/", "agent"},
		{"/guest/", "guest"},
		{"/admin/", "admin"},
	}

	for _, app := range apps {
		root := filepath.Join(staticDir, app.dir)
		if _, err := os.Stat(root); err != nil {
			continue
		}
		fs := http.FileServer(http.Dir(root))
		r.Handle(app.prefix+"*", http.StripPrefix(app.prefix, fs))
	}

	// Vite 多页构建共用 ../assets/，浏览器从站点根请求 /assets/*
	assetsRoot := filepath.Join(staticDir, "assets")
	if st, err := os.Stat(assetsRoot); err == nil && st.IsDir() {
		r.Handle("/assets/*", http.StripPrefix("/assets/", http.FileServer(http.Dir(assetsRoot))))
	}

	// 根路径重定向到坐席入口，便于内网直接访问主机名。
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/agent/", http.StatusFound)
	})
}
