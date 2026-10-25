package resourceapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rcourtman/pulse-go-rewrite/internal/api/apicontext"
	"github.com/rcourtman/pulse-go-rewrite/internal/config"
	"github.com/rcourtman/pulse-go-rewrite/internal/models"
	"github.com/rcourtman/pulse-go-rewrite/internal/unifiedresources"
)

type operatorRelationshipRecordProvider struct {
	records []unifiedresources.IngestRecord
}

func (p operatorRelationshipRecordProvider) GetCurrentRecords() []unifiedresources.IngestRecord {
	return append([]unifiedresources.IngestRecord(nil), p.records...)
}

func (p operatorRelationshipRecordProvider) SnapshotOwnedSources() []unifiedresources.DataSource {
	return []unifiedresources.DataSource{unifiedresources.SourceKomodo}
}

func TestOperatorRelationshipResourceAPIContract(t *testing.T) {
	const orgID = "default"
	serviceSourceID := "komodo:connection:server:jellyfin"
	dependencySourceID := "komodo:connection:deployment:database"
	serviceID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeKomodoServer, unifiedresources.SourceKomodo, serviceSourceID)
	dependencyID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeKomodoDeployment, unifiedresources.SourceKomodo, dependencySourceID)
	h := NewQueryService(&config.Config{DataPath: t.TempDir()})
	h.SetSupplementalRecordsProvider(unifiedresources.SourceKomodo, operatorRelationshipRecordProvider{records: []unifiedresources.IngestRecord{
		{SourceID: serviceSourceID, Resource: unifiedresources.Resource{Name: "Jellyfin", Type: unifiedresources.ResourceTypeKomodoServer}},
		{SourceID: dependencySourceID, Resource: unifiedresources.Resource{Name: "Database", Type: unifiedresources.ResourceTypeKomodoDeployment}},
	}})

	post := func() *httptest.ResponseRecorder {
		t.Helper()
		payload, _ := json.Marshal(map[string]string{"targetId": dependencyID, "type": "depends_on", "reason": "Jellyfin uses the database"})
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/resources/"+serviceID+"/relationships", bytes.NewReader(payload))
		request = request.WithContext(context.WithValue(request.Context(), apicontext.OrgIDContextKey, orgID))
		h.HandleResourceRoutes(recorder, request)
		return recorder
	}
	created := post()
	if created.Code != http.StatusCreated {
		t.Fatalf("POST relationship status=%d body=%s", created.Code, created.Body.String())
	}
	var createPayload struct {
		Relationship unifiedresources.OperatorResourceRelationship `json:"relationship"`
	}
	if err := json.NewDecoder(created.Body).Decode(&createPayload); err != nil {
		t.Fatal(err)
	}
	if createPayload.Relationship.SourceID != serviceID || createPayload.Relationship.TargetID != dependencyID || createPayload.Relationship.Type != unifiedresources.RelDependsOn {
		t.Fatalf("created relationship = %+v", createPayload.Relationship)
	}

	registry, err := h.BuildRegistry(orgID)
	if err != nil {
		t.Fatal(err)
	}
	service, ok := registry.Get(serviceID)
	if !ok || len(service.Relationships) != 1 || service.Relationships[0].Discoverer != "operator_store" {
		t.Fatalf("registry projection = %+v, found=%t", service, ok)
	}

	get := httptest.NewRecorder()
	getRequest := httptest.NewRequest(http.MethodGet, "/api/resources/"+serviceID+"/relationships", nil)
	getRequest = getRequest.WithContext(context.WithValue(getRequest.Context(), apicontext.OrgIDContextKey, orgID))
	h.HandleResourceRoutes(get, getRequest)
	if get.Code != http.StatusOK || !bytes.Contains(get.Body.Bytes(), []byte(`"depends_on"`)) {
		t.Fatalf("GET relationships status=%d body=%s", get.Code, get.Body.String())
	}

	deletePayload, _ := json.Marshal(map[string]string{"targetId": dependencyID, "type": "depends_on"})
	deleted := httptest.NewRecorder()
	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/resources/"+serviceID+"/relationships", bytes.NewReader(deletePayload))
	deleteRequest = deleteRequest.WithContext(context.WithValue(deleteRequest.Context(), apicontext.OrgIDContextKey, orgID))
	h.HandleResourceRoutes(deleted, deleteRequest)
	if deleted.Code != http.StatusOK {
		t.Fatalf("DELETE relationship status=%d body=%s", deleted.Code, deleted.Body.String())
	}
}

func TestHandleLinkRejectsDistinctResourcesFromSameProvider(t *testing.T) {
	firstSourceID := "komodo:connection:server:first"
	secondSourceID := "komodo:connection:server:second"
	firstID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeKomodoServer, unifiedresources.SourceKomodo, firstSourceID)
	secondID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeKomodoServer, unifiedresources.SourceKomodo, secondSourceID)
	h := NewQueryService(&config.Config{DataPath: t.TempDir()})
	provider := operatorRelationshipRecordProvider{records: []unifiedresources.IngestRecord{
		{SourceID: firstSourceID, Resource: unifiedresources.Resource{Name: "same name", Type: unifiedresources.ResourceTypeKomodoServer}},
		{SourceID: secondSourceID, Resource: unifiedresources.Resource{Name: "same name", Type: unifiedresources.ResourceTypeKomodoServer}},
	}}
	seed := unifiedresources.NewRegistry(nil)
	seed.IngestRecords(unifiedresources.SourceKomodo, provider.records)
	serialized, err := json.Marshal(seed.List())
	if err != nil {
		t.Fatal(err)
	}
	var rehydrated []unifiedresources.Resource
	if err := json.Unmarshal(serialized, &rehydrated); err != nil {
		t.Fatal(err)
	}
	h.SetStateProvider(resourceUnifiedSeedProvider{resources: rehydrated})
	h.SetSupplementalRecordsProvider(unifiedresources.SourceKomodo, provider)
	payload, _ := json.Marshal(map[string]string{"targetId": secondID, "reason": "test collision"})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/resources/"+firstID+"/link", bytes.NewReader(payload))
	h.HandleResourceRoutes(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("same-provider link status=%d body=%s, want 409", recorder.Code, recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte("komodo")) || !bytes.Contains(recorder.Body.Bytes(), []byte(firstSourceID)) || !bytes.Contains(recorder.Body.Bytes(), []byte(secondSourceID)) {
		t.Fatalf("conflict response does not identify provider and source IDs: %s", recorder.Body.String())
	}
	confirmed, _ := json.Marshal(map[string]any{
		"targetId": secondID, "reason": "operator confirmed these records identify one server", "confirmProviderConflict": true,
	})
	confirmedRecorder := httptest.NewRecorder()
	confirmedRequest := httptest.NewRequest(http.MethodPost, "/api/resources/"+firstID+"/link", bytes.NewReader(confirmed))
	h.HandleResourceRoutes(confirmedRecorder, confirmedRequest)
	if confirmedRecorder.Code != http.StatusOK {
		t.Fatalf("explicitly confirmed same-provider link status=%d body=%s, want 200", confirmedRecorder.Code, confirmedRecorder.Body.String())
	}
}

func TestOperatorRelationshipRoutesFollowLinkedCanonicalEndpoints(t *testing.T) {
	const orgID = "default"
	now := time.Now().UTC()
	proxmoxSourceID := "pve:cluster-a:vm:101"
	komodoSourceID := "komodo:connection-a:server:jellyfin"
	vmID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeVM, unifiedresources.SourceProxmox, proxmoxSourceID)
	serverID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeKomodoServer, unifiedresources.SourceKomodo, komodoSourceID)
	h := NewQueryService(&config.Config{DataPath: t.TempDir()})
	h.SetStateProvider(resourceUnifiedSeedProvider{
		snapshot: models.StateSnapshot{LastUpdate: now},
		resources: []unifiedresources.Resource{{
			ID: vmID, Type: unifiedresources.ResourceTypeVM, Name: "Jellyfin host", LastSeen: now,
			Sources: []unifiedresources.DataSource{unifiedresources.SourceProxmox},
			Proxmox: &unifiedresources.ProxmoxData{SourceID: proxmoxSourceID, VMID: 101},
		}},
	})
	h.SetSupplementalRecordsProvider(unifiedresources.SourceKomodo, operatorRelationshipRecordProvider{records: []unifiedresources.IngestRecord{{
		SourceID: komodoSourceID,
		Resource: unifiedresources.Resource{Name: "Jellyfin host", Type: unifiedresources.ResourceTypeKomodoServer, LastSeen: now},
	}}})
	store, err := h.getStore(orgID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddLink(unifiedresources.ResourceLink{ResourceA: vmID, ResourceB: serverID, PrimaryID: vmID}); err != nil {
		t.Fatal(err)
	}
	relationshipStore := store.(unifiedresources.OperatorRelationshipStore)
	edge := unifiedresources.OperatorResourceRelationship{
		SourceID: serverID, TargetID: "service:database", Type: unifiedresources.RelDependsOn,
		Reason: "original reason", CreatedBy: "operator:alice", UpdatedBy: "operator:alice",
	}
	if err := relationshipStore.PutOperatorRelationship(edge); err != nil {
		t.Fatal(err)
	}

	request := func(method, sourceID string, payload map[string]string) *httptest.ResponseRecorder {
		t.Helper()
		var body *bytes.Reader
		if payload == nil {
			body = bytes.NewReader(nil)
		} else {
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			body = bytes.NewReader(encoded)
		}
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/api/resources/"+sourceID+"/relationships", body)
		req = req.WithContext(context.WithValue(req.Context(), apicontext.OrgIDContextKey, orgID))
		h.HandleResourceRoutes(recorder, req)
		return recorder
	}
	getCanonical := request(http.MethodGet, vmID, nil)
	var response struct {
		Relationships []unifiedresources.OperatorResourceRelationship `json:"relationships"`
	}
	if getCanonical.Code != http.StatusOK {
		t.Fatalf("GET canonical linked source status=%d body=%s", getCanonical.Code, getCanonical.Body.String())
	}
	if err := json.NewDecoder(getCanonical.Body).Decode(&response); err != nil {
		t.Fatalf("decode canonical relationship response: %v", err)
	}
	if len(response.Relationships) != 1 {
		t.Fatalf("canonical relationship response = %+v, want one edge", response.Relationships)
	}
	got := response.Relationships[0]
	if got.ID != unifiedresources.OperatorRelationshipID(serverID, "service:database", unifiedresources.RelDependsOn) ||
		got.SourceID != vmID || got.TargetID != "service:database" || got.Reason != "original reason" {
		t.Fatalf("projected linked edge = %+v", got)
	}

	updated := request(http.MethodPost, vmID, map[string]string{
		"targetId": "service:database", "type": "depends_on", "reason": "updated through canonical VM",
	})
	if updated.Code != http.StatusCreated {
		t.Fatalf("POST canonical linked edge status=%d body=%s", updated.Code, updated.Body.String())
	}
	stored, err := relationshipStore.ListOperatorRelationships()
	if err != nil || len(stored) != 1 || stored[0].SourceID != serverID || stored[0].Reason != "updated through canonical VM" {
		t.Fatalf("POST created a duplicate or changed the stored identity: %+v err=%v", stored, err)
	}

	deleted := request(http.MethodDelete, vmID, map[string]string{"targetId": "service:database", "type": "depends_on"})
	if deleted.Code != http.StatusOK {
		t.Fatalf("DELETE canonical linked edge status=%d body=%s", deleted.Code, deleted.Body.String())
	}
	if stored, err = relationshipStore.ListOperatorRelationships(); err != nil || len(stored) != 0 {
		t.Fatalf("DELETE left active edge %+v err=%v", stored, err)
	}
}
