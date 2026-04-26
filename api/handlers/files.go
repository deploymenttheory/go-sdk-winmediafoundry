package handlers

import (
	"net/http"
	"strconv"

	"github.com/deploymenttheory/go-sdk-windowsuup/api/response"
	"github.com/deploymenttheory/go-sdk-windowsuup/winupdate"
	"github.com/go-chi/chi/v5"
)

// FilesHandler handles file metadata endpoints.
type FilesHandler struct {
	svc *winupdate.Service
}

// NewFilesHandler creates a FilesHandler.
func NewFilesHandler(svc *winupdate.Service) *FilesHandler {
	return &FilesHandler{svc: svc}
}

// List handles GET /v1/builds/{uuid}/files.
//
// Query parameters:
//
//	with_urls=<true>   resolve live CDN download URLs (calls GetExtendedUpdateInfo2)
//	revision=<int>     required when with_urls=true
func (h *FilesHandler) List(w http.ResponseWriter, r *http.Request) {
	uuid := chi.URLParam(r, "uuid")
	withURLs := r.URL.Query().Get("with_urls") == "true"

	revision := 0
	if s := r.URL.Query().Get("revision"); s != "" {
		revision, _ = strconv.Atoi(s)
	}

	files, err := h.svc.GetFiles(r.Context(), winupdate.GetFilesRequest{
		UUID:     uuid,
		WithURLs: withURLs,
		Revision: revision,
	})
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	if files == nil {
		files = []winupdate.FileResult{}
	}
	response.OK(w, files)
}
