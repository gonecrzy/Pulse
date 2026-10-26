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

type availabilityRelationshipRecordProvider struct {
	records []unifiedresources.IngestRecord
}

type tenantCanonicalResourcesProvider map[string][]unifiedresources.Resource

func (p tenantCanonicalResourcesProvider) UnifiedReadStateForTenant(string) unifiedresources.ReadState {
	return nil
}

func (p tenantCanonicalResourcesProvider) UnifiedResourceSnapshotForTenant(orgID string) ([]unifiedresources.Resource, time.Time) {
	return append([]unifiedresources.Resource(nil), p[orgID]...), time.Now().UTC()
}

func (p availabilityRelationshipRecordProvider) GetCurrentRecords() []unifiedresources.IngestRecord {
	return append([]unifiedresources.IngestRecord(nil), p.records...)
}

func (p availabilityRelationshipRecordProvider) SnapshotOwnedSources() []unifiedresources.DataSource {
	return []unifiedresources.DataSource{unifiedresources.SourceAvailability}
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

func TestResourceDetailsAndFacetsIncludeInboundCanonicalRelationships(t *testing.T) {
	const orgID = "default"
	now := time.Now().UTC()
	serverSourceID := "komodo:server:www-dmz"
	stackSourceID := "komodo:stack:kristi-www"
	checkSourceID := "availability:kristi-www-site"
	serverID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeKomodoServer, unifiedresources.SourceKomodo, serverSourceID)
	stackID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeKomodoStack, unifiedresources.SourceKomodo, stackSourceID)
	checkID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeNetworkEndpoint, unifiedresources.SourceAvailability, checkSourceID)
	komodoRecords := []unifiedresources.IngestRecord{
		{SourceID: serverSourceID, Resource: unifiedresources.Resource{ID: serverSourceID, Name: "www-DMZ", Type: unifiedresources.ResourceTypeKomodoServer, Status: unifiedresources.StatusOnline, LastSeen: now}},
		{SourceID: stackSourceID, ParentSourceID: serverSourceID, Resource: unifiedresources.Resource{ID: stackSourceID, Name: "kristi-www", Type: unifiedresources.ResourceTypeKomodoStack, Status: unifiedresources.StatusOnline, LastSeen: now}},
	}
	availabilityRecords := []unifiedresources.IngestRecord{{
		SourceID: checkSourceID,
		Resource: unifiedresources.Resource{ID: checkSourceID, Name: "kristi-www-site", Type: unifiedresources.ResourceTypeNetworkEndpoint, Status: unifiedresources.StatusOnline, LastSeen: now, Availability: &unifiedresources.AvailabilityData{
			TargetID: checkSourceID, LinkedResourceID: stackID, Protocol: "https", Address: "192.0.2.110", Enabled: true, Available: true, LastChecked: &now,
		}},
	}}
	h := NewQueryService(&config.Config{DataPath: t.TempDir()})
	h.SetSupplementalRecordsProvider(unifiedresources.SourceKomodo, operatorRelationshipRecordProvider{records: komodoRecords})
	h.SetSupplementalRecordsProvider(unifiedresources.SourceAvailability, operatorRelationshipRecordProvider{records: availabilityRecords})

	getResource := func(resourceID string) unifiedresources.Resource {
		t.Helper()
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/resources/"+resourceID, nil)
		request = request.WithContext(context.WithValue(request.Context(), apicontext.OrgIDContextKey, orgID))
		h.HandleResourceRoutes(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d body=%s", resourceID, recorder.Code, recorder.Body.String())
		}
		var resource unifiedresources.Resource
		if err := json.NewDecoder(recorder.Body).Decode(&resource); err != nil {
			t.Fatalf("decode resource %s: %v", resourceID, err)
		}
		return resource
	}
	server := getResource(serverID)
	if len(server.Relationships) != 1 || server.Relationships[0].SourceID != stackID || server.Relationships[0].TargetID != serverID || server.Relationships[0].Type != unifiedresources.RelHostedBy {
		t.Fatalf("server detail relationships = %+v, want inbound Stack hosted_by edge", server.Relationships)
	}
	stack := getResource(stackID)
	if len(stack.Relationships) != 2 {
		t.Fatalf("Stack detail relationships = %+v, want hosting and check associations", stack.Relationships)
	}
	var hasCheckEdge bool
	for _, relationship := range stack.Relationships {
		if relationship.SourceID == checkID && relationship.TargetID == stackID && relationship.Type == unifiedresources.RelChecks {
			hasCheckEdge = true
		}
	}
	if !hasCheckEdge {
		t.Fatalf("Stack detail relationships omit inbound availability check %q: %+v", checkID, stack.Relationships)
	}

	facets := httptest.NewRecorder()
	facetsRequest := httptest.NewRequest(http.MethodGet, "/api/resources/"+stackID+"/facets", nil)
	facetsRequest = facetsRequest.WithContext(context.WithValue(facetsRequest.Context(), apicontext.OrgIDContextKey, orgID))
	h.HandleResourceRoutes(facets, facetsRequest)
	if facets.Code != http.StatusOK {
		t.Fatalf("GET Stack facets status=%d body=%s", facets.Code, facets.Body.String())
	}
	var payload struct {
		Relationships []unifiedresources.ResourceRelationship `json:"relationships"`
	}
	if err := json.NewDecoder(facets.Body).Decode(&payload); err != nil {
		t.Fatalf("decode Stack facets: %v", err)
	}
	if len(payload.Relationships) != 2 {
		t.Fatalf("Stack facet relationships = %+v, want both-direction canonical edges", payload.Relationships)
	}
}

func TestProxmoxDetailsProjectRelatedKomodoChainAtTwoHops(t *testing.T) {
	const orgID = "default"
	now := time.Now().UTC()
	pveSourceID := "pve:cluster-a:vmid:50110"
	serverSourceID := "komodo:connection-a:server:www-dmz"
	stackSourceID := "komodo:connection-a:stack:kristi-www"
	checkSourceID := "kristi-www-site"
	vmID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeVM, unifiedresources.SourceProxmox, pveSourceID)
	serverID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeKomodoServer, unifiedresources.SourceKomodo, serverSourceID)
	stackID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeKomodoStack, unifiedresources.SourceKomodo, stackSourceID)
	checkID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeNetworkEndpoint, unifiedresources.SourceAvailability, checkSourceID)
	h := NewQueryService(&config.Config{DataPath: t.TempDir()})
	h.SetStateProvider(resourceUnifiedSeedProvider{resources: []unifiedresources.Resource{{
		ID: vmID, Type: unifiedresources.ResourceTypeVM, Name: "www-DMZ", Status: unifiedresources.StatusOnline, LastSeen: now,
		Sources: []unifiedresources.DataSource{unifiedresources.SourceProxmox},
		Proxmox: &unifiedresources.ProxmoxData{SourceID: pveSourceID, VMID: 50110, Instance: "cluster-a"},
	}}})
	h.SetSupplementalRecordsProvider(unifiedresources.SourceKomodo, operatorRelationshipRecordProvider{records: []unifiedresources.IngestRecord{
		{SourceID: serverSourceID, Resource: unifiedresources.Resource{ID: serverSourceID, Name: "www-DMZ", Type: unifiedresources.ResourceTypeKomodoServer, Status: unifiedresources.StatusOnline, LastSeen: now}},
		{SourceID: stackSourceID, ParentSourceID: serverSourceID, Resource: unifiedresources.Resource{ID: stackSourceID, Name: "kristi-www", Type: unifiedresources.ResourceTypeKomodoStack, Status: unifiedresources.StatusOnline, LastSeen: now}},
	}})
	checkedAt := now.Add(-30 * time.Second)
	h.SetSupplementalRecordsProvider(unifiedresources.SourceAvailability, availabilityRelationshipRecordProvider{records: []unifiedresources.IngestRecord{{
		SourceID: checkSourceID,
		Resource: unifiedresources.Resource{ID: checkSourceID, Name: "kristi-www-site", Type: unifiedresources.ResourceTypeNetworkEndpoint, Status: unifiedresources.StatusOffline, LastSeen: now, Availability: &unifiedresources.AvailabilityData{
			TargetID: checkSourceID, LinkedResourceID: stackID, Address: "192.0.2.110", Protocol: "https", Enabled: true, Available: false, LastChecked: &checkedAt,
		}},
	}}})
	store, err := h.getStore(orgID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddLink(unifiedresources.ResourceLink{ResourceA: vmID, ResourceB: serverID, PrimaryID: vmID, Reason: "fixture identity link"}); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/resources/"+vmID, nil)
	request = request.WithContext(context.WithValue(request.Context(), apicontext.OrgIDContextKey, orgID))
	response := httptest.NewRecorder()
	h.HandleResourceRoutes(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET VM details status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		ID                  string                             `json:"id"`
		Sources             []unifiedresources.DataSource      `json:"sources"`
		Canonical           unifiedresources.CanonicalIdentity `json:"canonicalIdentity"`
		RelationshipContext struct {
			Resources []struct {
				ID           string                                                        `json:"id"`
				Type         unifiedresources.ResourceType                                 `json:"type"`
				Name         string                                                        `json:"name"`
				Status       unifiedresources.ResourceStatus                               `json:"status"`
				LastSeen     time.Time                                                     `json:"lastSeen"`
				Sources      []unifiedresources.DataSource                                 `json:"sources"`
				SourceStatus map[unifiedresources.DataSource]unifiedresources.SourceStatus `json:"sourceStatus"`
				Availability *unifiedresources.AvailabilityData                            `json:"availability"`
			} `json:"resources"`
			Relationships []unifiedresources.ResourceRelationship `json:"relationships"`
		} `json:"relationshipContext"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.ID != vmID || payload.Canonical.PrimaryID != "vm:"+pveSourceID {
		t.Fatalf("VM identity = %q / %+v, want native Proxmox primary vm:%s", payload.ID, payload.Canonical, pveSourceID)
	}
	if !containsDataSource(payload.Sources, unifiedresources.SourceProxmox) || !containsDataSource(payload.Sources, unifiedresources.SourceKomodo) {
		t.Fatalf("VM provider sources = %v, want Proxmox and Komodo identity observations", payload.Sources)
	}
	contextIDs := make(map[string]struct{}, len(payload.RelationshipContext.Resources))
	for _, resource := range payload.RelationshipContext.Resources {
		contextIDs[resource.ID] = struct{}{}
		if resource.ID == stackID {
			observation := resource.SourceStatus[unifiedresources.SourceKomodo]
			if observation.Status != "online" || !observation.LastSeen.Equal(now) {
				t.Fatalf("Stack provider observation = %+v, want current Komodo state at %s", observation, now)
			}
		}
		if resource.ID == checkID {
			observation := resource.SourceStatus[unifiedresources.SourceAvailability]
			if resource.Availability == nil || resource.Availability.TargetID != checkSourceID || resource.Status != unifiedresources.StatusOffline || !resource.LastSeen.Equal(now) || !observation.LastSeen.Equal(now) {
				t.Fatalf("check context = %+v, want current failed HTTPS observation at %s", resource, now)
			}
		}
	}
	for _, id := range []string{vmID, stackID, checkID} {
		if _, ok := contextIDs[id]; !ok {
			t.Fatalf("relationship context omitted %q: %+v", id, payload.RelationshipContext.Resources)
		}
	}
	if len(payload.RelationshipContext.Resources) != 3 {
		t.Fatalf("relationship context has %d resources, want one each for VM, Stack, and check: %+v", len(payload.RelationshipContext.Resources), payload.RelationshipContext.Resources)
	}
	if len(payload.RelationshipContext.Relationships) != 2 {
		t.Fatalf("relationship context edges = %+v, want complete two-hop chain", payload.RelationshipContext.Relationships)
	}
	hosting, checks := false, false
	for _, relationship := range payload.RelationshipContext.Relationships {
		switch {
		case relationship.SourceID == stackID && relationship.TargetID == vmID && relationship.Type == unifiedresources.RelHostedBy:
			hosting = relationship.EvidenceSource == unifiedresources.SourceKomodo && relationship.Active
		case relationship.SourceID == checkID && relationship.TargetID == stackID && relationship.Type == unifiedresources.RelChecks:
			checks = relationship.EvidenceSource == unifiedresources.SourceAvailability && relationship.Active
		}
	}
	if !hosting || !checks {
		t.Fatalf("relationship context direction/provenance = %+v", payload.RelationshipContext.Relationships)
	}
}

func TestResourceRelationshipContextIsOrganizationScoped(t *testing.T) {
	const (
		orgA = "org-a"
		orgB = "org-b"
	)
	now := time.Now().UTC()
	vmID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeVM, unifiedresources.SourceProxmox, "pve:cluster-a:vmid:50110")
	stackID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeKomodoStack, unifiedresources.SourceKomodo, "komodo:connection-a:stack:kristi-www")
	resources := []unifiedresources.Resource{
		{ID: vmID, Type: unifiedresources.ResourceTypeVM, Name: "www-DMZ", Status: unifiedresources.StatusOnline, LastSeen: now, Sources: []unifiedresources.DataSource{unifiedresources.SourceProxmox}},
		{ID: stackID, Type: unifiedresources.ResourceTypeKomodoStack, Name: "kristi-www", Status: unifiedresources.StatusOnline, LastSeen: now, Sources: []unifiedresources.DataSource{unifiedresources.SourceKomodo}},
	}
	h := NewQueryService(&config.Config{DataPath: t.TempDir()})
	h.SetTenantStateProvider(tenantCanonicalResourcesProvider{orgA: resources, orgB: resources})
	storeA, err := h.getStore(orgA)
	if err != nil {
		t.Fatal(err)
	}
	relationshipStore, ok := storeA.(unifiedresources.OperatorRelationshipStore)
	if !ok {
		t.Fatal("organization resource store does not support operator relationships")
	}
	if err := relationshipStore.PutOperatorRelationship(unifiedresources.OperatorResourceRelationship{
		SourceID: vmID, TargetID: stackID, Type: unifiedresources.RelDependsOn,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("persist organization A relationship: %v", err)
	}

	read := func(orgID string) struct {
		Resources     []json.RawMessage                       `json:"resources"`
		Relationships []unifiedresources.ResourceRelationship `json:"relationships"`
	} {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/api/resources/"+vmID, nil)
		request = request.WithContext(context.WithValue(request.Context(), apicontext.OrgIDContextKey, orgID))
		response := httptest.NewRecorder()
		h.HandleResourceRoutes(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET VM for %s status=%d body=%s", orgID, response.Code, response.Body.String())
		}
		var payload struct {
			RelationshipContext struct {
				Resources     []json.RawMessage                       `json:"resources"`
				Relationships []unifiedresources.ResourceRelationship `json:"relationships"`
			} `json:"relationshipContext"`
		}
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			t.Fatalf("decode VM context for %s: %v", orgID, err)
		}
		return payload.RelationshipContext
	}
	contextA := read(orgA)
	if len(contextA.Resources) != 2 || len(contextA.Relationships) != 1 {
		t.Fatalf("organization A relationship context = %+v, want its VM-to-Stack edge", contextA)
	}
	contextB := read(orgB)
	if len(contextB.Resources) != 1 || len(contextB.Relationships) != 0 {
		t.Fatalf("organization B received organization A topology: %+v", contextB)
	}
}

func containsDataSource(sources []unifiedresources.DataSource, expected unifiedresources.DataSource) bool {
	for _, source := range sources {
		if source == expected {
			return true
		}
	}
	return false
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
	payload, _ := json.Marshal(map[string]any{"targetId": secondID, "reason": "test collision", "confirmIdentity": true})
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
		"targetId": secondID, "reason": "operator confirmed these records identify one server", "confirmIdentity": true, "confirmProviderConflict": true,
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
		resources: []unifiedresources.Resource{
			{
				ID: vmID, Type: unifiedresources.ResourceTypeVM, Name: "Jellyfin host", LastSeen: now,
				Sources: []unifiedresources.DataSource{unifiedresources.SourceProxmox},
				Proxmox: &unifiedresources.ProxmoxData{SourceID: proxmoxSourceID, VMID: 101},
			},
			{ID: "service:database", Type: unifiedresources.ResourceTypeKomodoDeployment, Name: "Database", LastSeen: now},
		},
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

func TestOperatorRelationshipAPIValidatesEndpointsAndDirection(t *testing.T) {
	h := NewQueryService(&config.Config{DataPath: t.TempDir()})
	h.SetStateProvider(resourceUnifiedSeedProvider{resources: []unifiedresources.Resource{
		{ID: "vm-a", Type: unifiedresources.ResourceTypeVM, Name: "VM"},
		{ID: "app-a", Type: unifiedresources.ResourceTypeAppContainer, Name: "App"},
		{ID: "stack-a", Type: unifiedresources.ResourceTypeKomodoStack, Name: "Stack"},
		{ID: "server-a", Type: unifiedresources.ResourceTypeKomodoServer, Name: "Server"},
	}})
	post := func(sourceID string, targetID, relationshipType string) *httptest.ResponseRecorder {
		t.Helper()
		payload, _ := json.Marshal(map[string]string{"targetId": targetID, "type": relationshipType})
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/resources/"+sourceID+"/relationships", bytes.NewReader(payload))
		request = request.WithContext(context.WithValue(request.Context(), apicontext.OrgIDContextKey, "default"))
		h.HandleResourceRoutes(recorder, request)
		return recorder
	}
	if got := post("vm-a", "missing-from-this-org", "depends_on"); got.Code != http.StatusNotFound {
		t.Fatalf("missing relationship target status=%d body=%s, want 404", got.Code, got.Body.String())
	}
	if got := post("vm-a", "app-a", "runs_on"); got.Code != http.StatusBadRequest {
		t.Fatalf("reversed runs_on status=%d body=%s, want 400", got.Code, got.Body.String())
	}
	if got := post("vm-a", "app-a", "routes_to"); got.Code != http.StatusBadRequest {
		t.Fatalf("unsupported routes_to pair status=%d body=%s, want 400", got.Code, got.Body.String())
	}
	if got := post("stack-a", "server-a", "hosted_by"); got.Code != http.StatusCreated {
		t.Fatalf("valid hosted_by status=%d body=%s, want 201", got.Code, got.Body.String())
	}
}

func TestHandleLinkRequiresExplicitIdentityConfirmation(t *testing.T) {
	h := NewQueryService(&config.Config{DataPath: t.TempDir()})
	h.SetStateProvider(resourceUnifiedSeedProvider{resources: []unifiedresources.Resource{
		{ID: "pve-vm", Type: unifiedresources.ResourceTypeVM, Name: "Jellyfin-DMZ", Sources: []unifiedresources.DataSource{unifiedresources.SourceProxmox}},
		{ID: "komodo-server", Type: unifiedresources.ResourceTypeKomodoServer, Name: "Jellyfin-DMZ", Sources: []unifiedresources.DataSource{unifiedresources.SourceKomodo}},
	}})
	post := func(confirm bool) *httptest.ResponseRecorder {
		t.Helper()
		payload, _ := json.Marshal(map[string]any{"targetId": "komodo-server", "confirmIdentity": confirm})
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/resources/pve-vm/link", bytes.NewReader(payload))
		request = request.WithContext(context.WithValue(request.Context(), apicontext.OrgIDContextKey, "default"))
		h.HandleResourceRoutes(recorder, request)
		return recorder
	}
	if got := post(false); got.Code != http.StatusBadRequest {
		t.Fatalf("unconfirmed identity link status=%d body=%s, want 400", got.Code, got.Body.String())
	}
	if got := post(true); got.Code != http.StatusOK {
		t.Fatalf("confirmed identity link status=%d body=%s, want 200", got.Code, got.Body.String())
	}
}

func TestIdentityLinkListIsOrganizationScoped(t *testing.T) {
	h := NewQueryService(&config.Config{DataPath: t.TempDir()})
	store, err := h.getStore("org-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddLink(unifiedresources.ResourceLink{
		ResourceA: "pve-vm", ResourceB: "komodo-server", PrimaryID: "pve-vm",
		Reason: "operator confirmed", CreatedBy: "operator:alice", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	read := func(orgID string) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/resources/identity-links", nil)
		request = request.WithContext(context.WithValue(request.Context(), apicontext.OrgIDContextKey, orgID))
		h.HandleResourceRoutes(recorder, request)
		return recorder
	}
	for _, tc := range []struct {
		orgID string
		want  int
	}{{"org-a", 1}, {"org-b", 0}} {
		response := read(tc.orgID)
		var body struct {
			Links []map[string]any `json:"links"`
		}
		if response.Code != http.StatusOK || json.NewDecoder(response.Body).Decode(&body) != nil || len(body.Links) != tc.want {
			t.Fatalf("identity links for %s: status=%d body=%s, want %d links", tc.orgID, response.Code, response.Body.String(), tc.want)
		}
	}
}
