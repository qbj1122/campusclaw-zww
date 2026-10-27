package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
)

func (s *Server) openapi(w http.ResponseWriter, r *http.Request) {
	b, err := os.ReadFile(filepath.Join("web", "openapi.json"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "OpenAPI 文档未生成")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

func (s *Server) swagger(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, filepath.Join("web", "swagger.html"))
}

func (s *Server) swaggerRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/swagger/", http.StatusFound)
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, filepath.Join("web", "index.html"))
}
