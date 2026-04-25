// Package handlers contains HTTP handlers for the Windows Update API.
package handlers

import (
	"net/http"
	"strconv"

	"github.com/deploymenttheory/go-sdk-uupdump/api/response"
	"github.com/deploymenttheory/go-sdk-uupdump/catalog"
	"github.com/deploymenttheory/go-sdk-uupdump/winupdate"
	"github.com/go-chi/chi/v5"
)

// BuildsHandler handles build-related HTTP endpoints.
type BuildsHandler struct {
	svc *winupdate.Service
}

// NewBuildsHandler creates a BuildsHandler.
func NewBuildsHandler(svc *winupdate.Service) *BuildsHandler {
	return &BuildsHandler{svc: svc}
}

// List handles GET /v1/builds.
//
// Query parameters:
//
//	search=<string>  substring filter on title
//	arch=<amd64|x86|arm64>
//	ring=<Dev|Beta|ReleasePreview|Retail>
//	stable=<true>    only stable builds
//	limit=<int>      default 50, max 500
//	offset=<int>
func (h *BuildsHandler) List(w http.ResponseWriter, r *http.Request) {
	q := catalog.BuildQuery{
		Search:     r.URL.Query().Get("search"),
		Arch:       r.URL.Query().Get("arch"),
		Ring:       r.URL.Query().Get("ring"),
		StableOnly: r.URL.Query().Get("stable") == "true",
		Limit:      parseIntParam(r, "limit", 50),
		Offset:     parseIntParam(r, "offset", 0),
		OrderBy:    r.URL.Query().Get("order"),
		Desc:       r.URL.Query().Get("desc") != "false",
	}

	builds, total, err := h.svc.ListBuilds(r.Context(), q)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	if builds == nil {
		builds = []catalog.Build{}
	}
	response.OKPaged(w, builds, total, q.Limit, q.Offset)
}

// Get handles GET /v1/builds/{uuid}.
func (h *BuildsHandler) Get(w http.ResponseWriter, r *http.Request) {
	uuid := chi.URLParam(r, "uuid")
	if uuid == "" {
		response.BadRequest(w, "uuid path parameter is required")
		return
	}
	build, err := h.svc.GetBuild(r.Context(), uuid)
	if err != nil {
		response.HandleStoreError(w, err)
		return
	}
	response.OK(w, build)
}

func parseIntParam(r *http.Request, key string, defaultVal int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return defaultVal
	}
	return v
}
