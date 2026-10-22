package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"kamacu/internal/artifact"
)

// ArtifactRoutes keeps upload control token-gated while sharing the existing
// loopback/origin boundary for UI operations. Upload bytes use a scoped token.
func ArtifactRoutes(mux *http.ServeMux, service *artifact.Service, token string) {
	h := &artifactHandlers{service: service, token: token}
	mux.HandleFunc("GET /api/tasks/{id}/artifacts", h.list)
	mux.HandleFunc("POST /api/tasks/{id}/artifact-uploads", h.begin)
	mux.HandleFunc("POST /api/tasks/{id}/artifacts/{artifact}/uploads", h.begin)
	mux.HandleFunc("PUT /api/artifact-uploads/{upload}", h.upload)
	mux.HandleFunc("POST /api/tasks/{id}/artifact-uploads/{upload}/finalize", h.finalize)
	mux.HandleFunc("PATCH /api/tasks/{id}/artifacts/{artifact}", h.rename)
	mux.HandleFunc("DELETE /api/tasks/{id}/artifacts/{artifact}", h.remove)
	mux.HandleFunc("GET /api/artifact-content/{artifact}/{revision}/{file...}", h.content)
}

type artifactHandlers struct {
	service *artifact.Service
	token   string
}

func (h *artifactHandlers) authorized(w http.ResponseWriter, r *http.Request) bool {
	if h.token == "" || subtle.ConstantTimeCompare([]byte(h.token), []byte(r.Header.Get("X-Kamacu-Token"))) != 1 {
		writeError(w, 401, "unauthorized")
		return false
	}
	return true
}
func artifactError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, artifact.ErrNotFound):
		status = 404
	case errors.Is(err, artifact.ErrConflict):
		status = 409
	case errors.Is(err, artifact.ErrUnauthorized):
		status = 401
	}
	writeError(w, status, err.Error())
}
func artifactTask(r *http.Request) int64 {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id
}
func (h *artifactHandlers) list(w http.ResponseWriter, r *http.Request) {
	a, err := h.service.List(r.Context(), artifactTask(r))
	if err != nil {
		artifactError(w, err)
		return
	}
	writeJSON(w, 200, a)
}
func (h *artifactHandlers) begin(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(w, r) {
		return
	}
	var input struct {
		Title    string `json:"title"`
		Filename string `json:"filename"`
		Size     int64  `json:"size_bytes"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input); err != nil {
		writeError(w, 400, "invalid JSON body")
		return
	}
	u, err := h.service.Begin(r.Context(), artifactTask(r), r.PathValue("artifact"), input.Title, input.Filename, input.Size)
	if err != nil {
		artifactError(w, err)
		return
	}
	// Return a relative path: the MCP bridge resolves it using the configured,
	// agent-reachable base URL, never a client-controlled Host header.
	writeJSON(w, 201, map[string]any{"upload_id": u.ID, "upload_url": "/api/artifact-uploads/" + u.ID, "method": "PUT", "headers": map[string]string{"Authorization": "Bearer " + u.Token, "Content-Type": "application/octet-stream"}, "expires_at": u.ExpiresAt})
}
func (h *artifactHandlers) upload(w http.ResponseWriter, r *http.Request) {
	// Bound stalled clients independently of the 15-minute capability lifetime.
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(2 * time.Minute))
	defer controller.SetReadDeadline(time.Time{})
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if err := h.service.Receive(r.Context(), r.PathValue("upload"), token, http.MaxBytesReader(w, r.Body, artifact.MaxUpload)); err != nil {
		artifactError(w, err)
		return
	}
	w.WriteHeader(204)
}
func (h *artifactHandlers) finalize(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(w, r) {
		return
	}
	a, err := h.service.Finalize(r.Context(), artifactTask(r), r.PathValue("upload"))
	if err != nil {
		artifactError(w, err)
		return
	}
	writeJSON(w, 200, a)
}
func (h *artifactHandlers) rename(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Title string `json:"title"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, 400, "invalid JSON body")
		return
	}
	a, err := h.service.Rename(r.Context(), artifactTask(r), r.PathValue("artifact"), input.Title)
	if err != nil {
		artifactError(w, err)
		return
	}
	writeJSON(w, 200, a)
}
func (h *artifactHandlers) remove(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Delete(r.Context(), artifactTask(r), r.PathValue("artifact")); err != nil {
		artifactError(w, err)
		return
	}
	// Metadata deletion revokes access immediately; periodic collection retries failures.
	_ = h.service.Sweep(r.Context())
	w.WriteHeader(204)
}
func (h *artifactHandlers) content(w http.ResponseWriter, r *http.Request) {
	file := r.PathValue("file")
	f, err := h.service.Open(r.Context(), r.PathValue("artifact"), r.PathValue("revision"), file)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	contentType := mime.TypeByExtension(path.Ext(file))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	// Host is validated by the app's outer host-check middleware. This source
	// expression permits only this immutable revision, never other API paths.
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	source := fmt.Sprintf("%s://%s/api/artifact-content/%s/%s/", scheme, r.Host, r.PathValue("artifact"), r.PathValue("revision"))
	csp := fmt.Sprintf("sandbox allow-scripts; default-src 'none'; script-src 'unsafe-inline' %s; style-src 'unsafe-inline' %s; img-src data: blob: %s; font-src data: %s; media-src data: blob: %s; connect-src %s; frame-src 'none'; worker-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'self'", source, source, source, source, source, source)
	// Active asset formats such as SVG/XML must also be safe on direct navigation.
	// HTML alone executes the trusted capability gate before any uploaded code.
	isHTML := strings.HasPrefix(contentType, "text/html")
	if !isHTML {
		csp = strings.Replace(csp, "sandbox allow-scripts;", "sandbox;", 1)
		// Subresource loads (scripts, styles, images, fonts, JSON) still work,
		// but document navigation must not render an unverified SVG/XML/PDF
		// viewer with clickable links on browsers that ignore the native policy.
		w.Header().Set("Content-Disposition", "attachment")
	}
	w.Header().Set("Content-Security-Policy", csp)
	w.Header().Set("Connection-Allowlist", fmt.Sprintf("(%q);webrtc=block;redirects=block", source+"*"))
	w.Header().Set("X-DNS-Prefetch-Control", "off")
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	// Opaque-origin module scripts/fonts can read uploaded assets. No credentials
	// or CORS permissions are granted to application API endpoints.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
	if isHTML {
		if r.Method == http.MethodHead {
			return
		}
		if err := renderArtifactHTML(w, f, r.URL.Query().Get("preview_token")); err != nil {
			slog.Warn("rendering artifact preview", "artifact", r.PathValue("artifact"), "error", err)
		}
		return
	}
	http.ServeContent(w, r, file, info.ModTime(), f)
}
