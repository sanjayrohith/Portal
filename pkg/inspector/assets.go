package inspector

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed web/*
var webAssets embed.FS

func staticHandler() http.Handler {
	files, err := fs.Sub(webAssets, "web")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(files))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			index, readErr := fs.ReadFile(webAssets, "web/index.html")
			if readErr != nil {
				http.Error(w, "inspector UI unavailable", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(index)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/static/") {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/static")
			fileServer.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	})
}
