package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/rcourtman/pulse-go-rewrite/internal/forkfeatures"
	"github.com/rcourtman/pulse-go-rewrite/pkg/auth"
)

const maxForkProfileRequestBytes = 16 * 1024

type ForkFeatureHandlers struct {
	manager *forkfeatures.Manager
}

func NewForkFeatureHandlers(manager *forkfeatures.Manager) *ForkFeatureHandlers {
	return &ForkFeatureHandlers{manager: manager}
}

func (h *ForkFeatureHandlers) HandleCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	snapshot, err := h.manager.Snapshot()
	if err != nil {
		writeErrorResponse(w, http.StatusServiceUnavailable, "fork_capabilities_unavailable", "Fork capabilities are unavailable", nil)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(snapshot); err != nil {
		return
	}
}

func (h *ForkFeatureHandlers) HandleProfile(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		snapshot, err := h.manager.Snapshot()
		if err != nil {
			writeErrorResponse(w, http.StatusServiceUnavailable, "fork_profile_invalid", "Fork profile is invalid and must be replaced by an administrator", nil)
			return
		}
		profile := struct {
			Version         int                      `json:"version"`
			Revision        uint64                   `json:"revision"`
			EnabledFeatures []forkfeatures.FeatureID `json:"enabled_features"`
		}{Version: snapshot.ProfileVersion, Revision: snapshot.Revision, EnabledFeatures: []forkfeatures.FeatureID{}}
		for _, capability := range snapshot.Capabilities {
			if capability.Available {
				profile.EnabledFeatures = append(profile.EnabledFeatures, capability.ID)
			}
		}
		writeJSON(w, http.StatusOK, profile)
	case http.MethodPut:
		h.handleReplaceProfile(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *ForkFeatureHandlers) handleReplaceProfile(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxForkProfileRequestBytes))
	decoder.DisallowUnknownFields()
	var request struct {
		Version          int                      `json:"version"`
		ExpectedRevision uint64                   `json:"expected_revision"`
		EnabledFeatures  []forkfeatures.FeatureID `json:"enabled_features"`
	}
	if err := decoder.Decode(&request); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "invalid_fork_profile", "Fork profile body is invalid", nil)
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeErrorResponse(w, http.StatusBadRequest, "invalid_fork_profile", "Fork profile body must contain one JSON object", nil)
		return
	}
	profile := forkfeatures.ForkProfile{Version: request.Version, EnabledFeatures: request.EnabledFeatures}
	snapshot, err := h.manager.Replace(profile, request.ExpectedRevision)
	if err != nil {
		if errors.Is(err, forkfeatures.ErrStaleRevision) {
			writeErrorResponse(w, http.StatusConflict, "stale_fork_profile", "Fork profile changed; reload it before saving", nil)
			return
		}
		if errors.Is(err, forkfeatures.ErrInvalidProfile) {
			writeErrorResponse(w, http.StatusBadRequest, "invalid_fork_profile", err.Error(), nil)
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "fork_profile_save_failed", "Fork profile could not be saved", nil)
		return
	}
	LogAuditEventForTenant(GetOrgID(r.Context()), "fork_profile_updated", authUsername(r), GetClientIP(r), r.URL.Path, true,
		fmt.Sprintf("Updated personal-fork capability profile to revision %d", snapshot.Revision))
	writeJSON(w, http.StatusOK, snapshot)
}

func (r *Router) withForkCapability(id forkfeatures.FeatureID, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		decision, err := r.forkAccess.Require(req.Context(), id)
		if err != nil {
			writeErrorResponse(w, http.StatusForbidden, "fork_capability_unavailable", "Required fork capability is unavailable", map[string]string{
				"capability": string(id),
			})
			return
		}
		_ = decision // The resolver decision is intentionally centralized; this gate grants availability only.
		next(w, req)
	}
}

func authUsername(r *http.Request) string {
	return auth.GetUser(r.Context())
}
