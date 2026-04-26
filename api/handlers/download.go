package handlers

import (
	"net/http"
	"strconv"

	"github.com/deploymenttheory/go-sdk-windowsuup/api/response"
	"github.com/deploymenttheory/go-sdk-windowsuup/winupdate"
	"github.com/go-chi/chi/v5"
)

// DownloadHandler handles streaming CDN proxy downloads.
type DownloadHandler struct {
	svc *winupdate.Service
}

// NewDownloadHandler creates a DownloadHandler.
func NewDownloadHandler(svc *winupdate.Service) *DownloadHandler {
	return &DownloadHandler{svc: svc}
}

// Stream handles GET /v1/builds/{uuid}/files/{filename}/download.
//
// Query parameters:
//
//	revision=<int>  update revision number (required)
func (h *DownloadHandler) Stream(w http.ResponseWriter, r *http.Request) {
	uuid := chi.URLParam(r, "uuid")
	filename := chi.URLParam(r, "filename")

	revision := 0
	if s := r.URL.Query().Get("revision"); s != "" {
		revision, _ = strconv.Atoi(s)
	}
	if revision == 0 {
		response.BadRequest(w, "revision query parameter is required")
		return
	}

	n, contentType, err := h.svc.StreamDownload(r.Context(), winupdate.DownloadRequest{
		UpdateID: uuid,
		Revision: revision,
		Filename: filename,
	}, w)
	if err != nil {
		// If we haven't written the header yet, report the error properly.
		if n == 0 {
			response.InternalError(w, err.Error())
		}
		return
	}
	_ = contentType
	// Headers must be set BEFORE streaming starts; do it via wrapper in server.go.
}
