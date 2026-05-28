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
		serveEmbeddedAsset(w, dist, "index.html")
		return
	}
	serveEmbeddedAsset(w, dist, assetPath)
}

func serveEmbeddedAsset(w http.ResponseWriter, dist fs.FS, assetPath string) {
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
