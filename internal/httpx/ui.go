package httpx

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

//go:embed web_dist/*
var webDist embed.FS

func (s *Server) uiIndex(w http.ResponseWriter, r *http.Request) {
	s.serveUI(w, r)
}

func (s *Server) uiApp(w http.ResponseWriter, r *http.Request) {
	s.serveUI(w, r)
}

func (s *Server) serveUI(w http.ResponseWriter, r *http.Request) {
	dist, err := fs.Sub(webDist, "web_dist")
	if err != nil {
		http.Error(w, "ui assets not available", http.StatusInternalServerError)
		return
	}

	assetPath := strings.TrimPrefix(r.URL.Path, "/ui/")
	if assetPath == "" || assetPath == "/" || r.URL.Path == "/" {
		serveEmbeddedAsset(w, dist, "index.html")
		return
	}

	assetPath = path.Clean("/" + assetPath)[1:]
	if strings.HasPrefix(assetPath, "../") || assetPath == ".." || strings.HasPrefix(assetPath, ".") {
		http.NotFound(w, r)
		return
	}
	info, err := fs.Stat(dist, assetPath)
	if err != nil || info.IsDir() {
		// 静态构建资源缺失必须明确 404，避免浏览器把 index.html 当 JS/CSS 执行后掩盖发布问题。
		if strings.HasPrefix(assetPath, "assets/") {
			http.NotFound(w, r)
			return
		}
		serveEmbeddedAsset(w, dist, "index.html")
		return
	}
	serveEmbeddedAsset(w, dist, assetPath)
}

func serveEmbeddedAsset(w http.ResponseWriter, dist fs.FS, assetPath string) {
	if assetPath == "index.html" {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
	} else if strings.HasPrefix(assetPath, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	data, err := fs.ReadFile(dist, assetPath)
	if err != nil {
		http.Error(w, "ui asset not found", http.StatusNotFound)
		return
	}
	if ctype := mime.TypeByExtension(path.Ext(assetPath)); ctype != "" {
		w.Header().Set("Content-Type", ctype)
	} else if strings.HasSuffix(assetPath, ".js") {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	} else if strings.HasSuffix(assetPath, ".css") {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	}
	_, _ = w.Write(data)
}
