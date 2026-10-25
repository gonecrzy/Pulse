package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/rcourtman/pulse-go-rewrite/internal/config"
	"github.com/rcourtman/pulse-go-rewrite/internal/komodo"
	"github.com/rcourtman/pulse-go-rewrite/internal/mock"
	"github.com/rcourtman/pulse-go-rewrite/internal/monitoring"
)

const komodoConnectionsPathPrefix = "/api/komodo/connections/"

// KomodoHandlers owns org-scoped Komodo connection CRUD, connectivity tests,
// and redacted polling status.
type KomodoHandlers struct {
	getPersistence func(context.Context) *config.ConfigPersistence
	getPoller      func(context.Context) *monitoring.KomodoPoller
	newClient      func(config.KomodoInstance) (komodoInventoryClient, error)
}

type komodoInventoryClient interface {
	CollectInventory(context.Context) (komodo.Inventory, error)
}

type komodoConnectionResponse struct {
	config.KomodoInstance
	Poll *monitoring.KomodoConnectionPollStatus `json:"poll,omitempty"`
}

// HandleAdd stores a new encrypted Komodo connection.
func (h *KomodoHandlers) HandleAdd(w http.ResponseWriter, r *http.Request) {
	if mock.IsMockEnabled() {
		writeErrorResponse(w, http.StatusForbidden, "mock_mode_enabled", "Cannot modify connections in mock mode", nil)
		return
	}
	instance, ok := decodeKomodoInstanceRequest(w, r, config.NewKomodoInstance())
	if !ok {
		return
	}
	instance.ID = strings.TrimSpace(instance.ID)
	if instance.ID == "" {
		instance.ID = config.NewKomodoInstance().ID
	}
	normalizeKomodoInstance(&instance)
	if err := instance.Validate(); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	if err := h.verifyInstance(r.Context(), instance); err != nil {
		writeKomodoTestFailure(w, err)
		return
	}
	persistence := h.persistenceForRequest(w, r.Context())
	if persistence == nil {
		return
	}
	instances, err := persistence.LoadKomodoConfig()
	if err != nil {
		writeErrorResponse(w, http.StatusInternalServerError, "komodo_load_failed", "Failed to load Komodo configuration", nil)
		return
	}
	for _, existing := range instances {
		if existing.ID == instance.ID {
			writeErrorResponse(w, http.StatusConflict, "komodo_duplicate_id", "Connection ID already exists", nil)
			return
		}
	}
	instances = append(instances, instance)
	if err := persistence.SaveKomodoConfig(instances); err != nil {
		writeErrorResponse(w, http.StatusInternalServerError, "komodo_save_failed", "Failed to save Komodo configuration", nil)
		return
	}
	writeJSON(w, http.StatusCreated, instance.Redacted())
}

// HandleList returns Komodo configurations with credentials masked and the
// poller's org-scoped cached health.
func (h *KomodoHandlers) HandleList(w http.ResponseWriter, r *http.Request) {
	persistence := h.persistenceForRequest(w, r.Context())
	if persistence == nil {
		return
	}
	instances, err := persistence.LoadKomodoConfig()
	if err != nil {
		writeErrorResponse(w, http.StatusInternalServerError, "komodo_load_failed", "Failed to load Komodo configuration", nil)
		return
	}
	var summaries map[string]monitoring.KomodoConnectionPollStatus
	if h != nil && h.getPoller != nil {
		if poller := h.getPoller(r.Context()); poller != nil {
			summaries = poller.ConnectionSummaries(resolveTenantOrgID(r), instances)
		}
	}
	responses := make([]komodoConnectionResponse, 0, len(instances))
	for i := range instances {
		instance := instances[i]
		response := komodoConnectionResponse{KomodoInstance: instance.Redacted()}
		if summary, exists := summaries[strings.TrimSpace(instance.ID)]; exists {
			statusCopy := summary
			response.Poll = &statusCopy
		}
		responses = append(responses, response)
	}
	writeJSON(w, http.StatusOK, responses)
}

// HandleDelete removes one Komodo connection and its last-known poller cache
// on the next config sync.
func (h *KomodoHandlers) HandleDelete(w http.ResponseWriter, r *http.Request) {
	if mock.IsMockEnabled() {
		writeErrorResponse(w, http.StatusForbidden, "mock_mode_enabled", "Cannot modify connections in mock mode", nil)
		return
	}
	id, ok := komodoConnectionIDFromPath(r.URL.Path, false)
	if !ok {
		writeErrorResponse(w, http.StatusBadRequest, "missing_connection_id", "Connection ID is required", nil)
		return
	}
	persistence := h.persistenceForRequest(w, r.Context())
	if persistence == nil {
		return
	}
	instances, err := persistence.LoadKomodoConfig()
	if err != nil {
		writeErrorResponse(w, http.StatusInternalServerError, "komodo_load_failed", "Failed to load Komodo configuration", nil)
		return
	}
	index := -1
	for i := range instances {
		if strings.TrimSpace(instances[i].ID) == id {
			index = i
			break
		}
	}
	if index < 0 {
		writeErrorResponse(w, http.StatusNotFound, "komodo_not_found", "Connection not found", nil)
		return
	}
	instances = append(instances[:index], instances[index+1:]...)
	if err := persistence.SaveKomodoConfig(instances); err != nil {
		writeErrorResponse(w, http.StatusInternalServerError, "komodo_save_failed", "Failed to save Komodo configuration", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "id": id})
}

// HandleUpdate replaces a connection while preserving masked credentials.
func (h *KomodoHandlers) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	if mock.IsMockEnabled() {
		writeErrorResponse(w, http.StatusForbidden, "mock_mode_enabled", "Cannot modify connections in mock mode", nil)
		return
	}
	id, ok := komodoConnectionIDFromPath(r.URL.Path, false)
	if !ok {
		writeErrorResponse(w, http.StatusBadRequest, "missing_connection_id", "Connection ID is required", nil)
		return
	}
	persistence := h.persistenceForRequest(w, r.Context())
	if persistence == nil {
		return
	}
	instances, err := persistence.LoadKomodoConfig()
	if err != nil {
		writeErrorResponse(w, http.StatusInternalServerError, "komodo_load_failed", "Failed to load Komodo configuration", nil)
		return
	}
	updatePlatformConnection(w, r, id, instances, platformConnectionUpdateSpec[config.KomodoInstance]{
		notFoundCode:    "komodo_not_found",
		saveFailCode:    "komodo_save_failed",
		saveFailMsg:     "Failed to save Komodo configuration",
		preSaveFailCode: "komodo_connection_failed",
		preSaveFailMsg:  "Komodo credential or endpoint change could not be verified",
		id:              func(instance config.KomodoInstance) string { return instance.ID },
		decode:          decodeKomodoInstanceRequest,
		setID:           func(instance *config.KomodoInstance, id string) { instance.ID = id },
		normalize:       normalizeKomodoInstance,
		preserve: func(updated *config.KomodoInstance, stored config.KomodoInstance) {
			updated.PreserveMaskedSecrets(stored)
		},
		validate: func(instance config.KomodoInstance) error { return instance.Validate() },
		preSave: func(ctx context.Context, updated, stored config.KomodoInstance) error {
			if updated.Endpoint == stored.Endpoint && updated.APIKey == stored.APIKey && updated.APISecret == stored.APISecret {
				return nil
			}
			return h.verifyInstance(ctx, updated)
		},
		redacted: func(instance config.KomodoInstance) any { return instance.Redacted() },
		save:     persistence.SaveKomodoConfig,
	})
}

// HandleTestConnection verifies API authentication, Core major version,
// inventory endpoint shape, and read scope without saving the candidate.
func (h *KomodoHandlers) HandleTestConnection(w http.ResponseWriter, r *http.Request) {
	instance, ok := decodeKomodoInstanceRequest(w, r, config.NewKomodoInstance())
	if !ok {
		return
	}
	normalizeKomodoInstance(&instance)
	h.writeTestResult(w, r, instance)
}

// HandleTestSavedConnection tests the stored credentials for one connection.
func (h *KomodoHandlers) HandleTestSavedConnection(w http.ResponseWriter, r *http.Request) {
	id, ok := komodoConnectionIDFromPath(r.URL.Path, true)
	if !ok {
		writeErrorResponse(w, http.StatusBadRequest, "missing_connection_id", "Connection ID is required", nil)
		return
	}
	persistence := h.persistenceForRequest(w, r.Context())
	if persistence == nil {
		return
	}
	instances, err := persistence.LoadKomodoConfig()
	if err != nil {
		writeErrorResponse(w, http.StatusInternalServerError, "komodo_load_failed", "Failed to load Komodo configuration", nil)
		return
	}
	for _, instance := range instances {
		if strings.TrimSpace(instance.ID) == id {
			// The editor can submit candidate endpoint/credentials here. Masked
			// values preserve the saved secret; supplied values are tested without
			// saving or disclosing them.
			if r.ContentLength > 0 {
				candidate, ok := decodeKomodoInstanceRequest(w, r, instance)
				if !ok {
					return
				}
				candidate.ID = instance.ID
				candidate.PreserveMaskedSecrets(instance)
				normalizeKomodoInstance(&candidate)
				h.writeTestResult(w, r, candidate)
				return
			}
			h.writeTestResult(w, r, instance)
			return
		}
	}
	writeErrorResponse(w, http.StatusNotFound, "komodo_not_found", "Connection not found", nil)
}

func (h *KomodoHandlers) writeTestResult(w http.ResponseWriter, r *http.Request, instance config.KomodoInstance) {
	if err := instance.Validate(); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "komodo_invalid_config", "Invalid Komodo connection configuration", nil)
		return
	}
	if err := h.verifyInstance(r.Context(), instance); err != nil {
		writeKomodoTestFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *KomodoHandlers) verifyInstance(ctx context.Context, instance config.KomodoInstance) error {
	client, err := h.clientForInstance(instance)
	if err != nil {
		return errors.New("Komodo client could not be created")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if _, err := client.CollectInventory(ctx); err != nil {
		return err
	}
	return nil
}

func (h *KomodoHandlers) clientForInstance(instance config.KomodoInstance) (komodoInventoryClient, error) {
	if h != nil && h.newClient != nil {
		return h.newClient(instance)
	}
	client, err := komodo.NewClient(komodo.ClientConfig{Endpoint: instance.Endpoint, APIKey: instance.APIKey, APISecret: instance.APISecret})
	if err != nil {
		return nil, errors.New("Komodo client could not be created")
	}
	return client, nil
}

func writeKomodoTestFailure(w http.ResponseWriter, err error) {
	message := "Komodo inventory test failed"
	if err != nil {
		lower := strings.ToLower(err.Error())
		switch {
		case strings.Contains(lower, "auth"):
			message = "Komodo authentication failed"
		case strings.Contains(lower, "permission"), strings.Contains(lower, "scope"):
			message = "Komodo read permission scope could not be verified"
		case strings.Contains(lower, "version"):
			message = "Komodo Core API version is unsupported"
		case strings.Contains(lower, "timeout"):
			message = "Komodo API request timed out"
		}
	}
	writeErrorResponse(w, http.StatusBadRequest, "komodo_connection_failed", message, nil)
}

func decodeKomodoInstanceRequest(w http.ResponseWriter, r *http.Request, base config.KomodoInstance) (config.KomodoInstance, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
	defer r.Body.Close()
	instance := base
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&instance); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body", nil)
		return config.KomodoInstance{}, false
	}
	return instance, true
}

func normalizeKomodoInstance(instance *config.KomodoInstance) {
	if instance == nil {
		return
	}
	instance.ID = strings.TrimSpace(instance.ID)
	instance.Name = strings.TrimSpace(instance.Name)
	instance.Endpoint = strings.TrimSpace(instance.Endpoint)
	instance.APIKey = strings.TrimSpace(instance.APIKey)
	instance.APISecret = strings.TrimSpace(instance.APISecret)
}

func (h *KomodoHandlers) persistenceForRequest(w http.ResponseWriter, ctx context.Context) *config.ConfigPersistence {
	if h == nil || h.getPersistence == nil {
		writeErrorResponse(w, http.StatusInternalServerError, "komodo_unavailable", "Komodo service unavailable", nil)
		return nil
	}
	persistence := h.getPersistence(ctx)
	if persistence == nil {
		writeErrorResponse(w, http.StatusInternalServerError, "komodo_unavailable", "Komodo service unavailable", nil)
		return nil
	}
	return persistence
}

func komodoConnectionIDFromPath(path string, requireTest bool) (string, bool) {
	if !strings.HasPrefix(path, komodoConnectionsPathPrefix) {
		return "", false
	}
	parts := strings.Split(strings.TrimPrefix(path, komodoConnectionsPathPrefix), "/")
	if requireTest {
		if len(parts) != 2 || parts[1] != "test" || strings.TrimSpace(parts[0]) == "" {
			return "", false
		}
		return parts[0], true
	}
	if len(parts) != 1 || strings.TrimSpace(parts[0]) == "" {
		return "", false
	}
	return parts[0], true
}
