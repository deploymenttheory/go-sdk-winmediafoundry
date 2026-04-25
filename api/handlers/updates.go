package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploymenttheory/go-sdk-uupdump/api/response"
	"github.com/deploymenttheory/go-sdk-uupdump/winupdate"
	"github.com/deploymenttheory/go-sdk-uupdump/wuproto"
)

// UpdatesHandler handles live Windows Update fetch requests.
type UpdatesHandler struct {
	svc *winupdate.Service
}

// NewUpdatesHandler creates an UpdatesHandler.
func NewUpdatesHandler(svc *winupdate.Service) *UpdatesHandler {
	return &UpdatesHandler{svc: svc}
}

type fetchRequest struct {
	Arch       string `json:"arch"`
	Ring       string `json:"ring"`
	Flight     string `json:"flight"`
	Build      string `json:"build"`
	CheckBuild string `json:"check_build"`
	SKU        int    `json:"sku"`
}

// Fetch handles POST /v1/updates/fetch.
// Triggers a live SyncUpdates SOAP call and stores the results.
func (h *UpdatesHandler) Fetch(w http.ResponseWriter, r *http.Request) {
	var req fetchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid JSON request body: "+err.Error())
		return
	}

	if req.Arch == "" {
		req.Arch = "amd64"
	}
	if req.Ring == "" {
		req.Ring = "Retail"
	}
	if req.Flight == "" {
		req.Flight = "Active"
	}

	result, err := h.svc.FetchAndStore(r.Context(), winupdate.FetchAndStoreRequest{
		Arch:       wuproto.Arch(req.Arch),
		Ring:       wuproto.Ring(req.Ring),
		Flight:     wuproto.Flight(req.Flight),
		Build:      req.Build,
		CheckBuild: req.CheckBuild,
		SKU:        req.SKU,
		Type:       wuproto.BuildTypeProduction,
	})
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.OK(w, result)
}
