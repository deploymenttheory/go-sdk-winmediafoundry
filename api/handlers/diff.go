package handlers

import (
	"net/http"

	"github.com/deploymenttheory/go-sdk-uupdump/api/response"
	"github.com/deploymenttheory/go-sdk-uupdump/winupdate"
)

// DiffHandler handles build comparison requests.
type DiffHandler struct {
	svc *winupdate.Service
}

// NewDiffHandler creates a DiffHandler.
func NewDiffHandler(svc *winupdate.Service) *DiffHandler {
	return &DiffHandler{svc: svc}
}

// Compare handles GET /v1/diff?base={uuid}&target={uuid}.
func (h *DiffHandler) Compare(w http.ResponseWriter, r *http.Request) {
	base := r.URL.Query().Get("base")
	target := r.URL.Query().Get("target")

	if base == "" || target == "" {
		response.BadRequest(w, "both 'base' and 'target' query parameters are required")
		return
	}
	if base == target {
		response.BadRequest(w, "'base' and 'target' must be different UUIDs")
		return
	}

	diff, err := h.svc.Diff(r.Context(), base, target)
	if err != nil {
		response.HandleStoreError(w, err)
		return
	}
	response.OK(w, diff)
}
