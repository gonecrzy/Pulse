package resourceapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
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

func TestResourceDetailsSeparateServiceMonitoringFromSharedHostContext(t *testing.T) {
	const orgID = "default"
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	const (
		instance              = "cluster-a"
		nodeName              = "pve1"
		vmSourceID            = "pve:cluster-a:vmid:50110"
		siblingSource         = "pve:cluster-a:vmid:50111"
		serverSourceID        = "komodo:connection-a:server:www-dmz"
		siblingServerSourceID = "komodo:connection-a:server:alfheim"
		stackSourceID         = "komodo:connection-a:stack:kristi-www"
		stack2SourceID        = "komodo:connection-a:stack:metrics"
		siblingStackSourceID  = "komodo:connection-a:stack:alfheim-main"
		siblingStack2SourceID = "komodo:connection-a:stack:alfheim-worker"
		checkSourceID         = "https-kristi-www"
		check2SourceID        = "https-metrics"
		siblingCheckSourceID  = "https-alfheim-main"
		siblingCheck2SourceID = "https-alfheim-worker"
	)
	stackLinkID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeKomodoStack, unifiedresources.SourceKomodo, stackSourceID)
	stack2LinkID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeKomodoStack, unifiedresources.SourceKomodo, stack2SourceID)
	serverLinkID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeKomodoServer, unifiedresources.SourceKomodo, serverSourceID)
	siblingServerLinkID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeKomodoServer, unifiedresources.SourceKomodo, siblingServerSourceID)
	siblingStackLinkID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeKomodoStack, unifiedresources.SourceKomodo, siblingStackSourceID)
	siblingStack2LinkID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeKomodoStack, unifiedresources.SourceKomodo, siblingStack2SourceID)
	checkedAt := now.Add(-time.Minute)
	state := models.StateSnapshot{
		LastUpdate: now,
		Nodes:      []models.Node{{ID: instance + "-" + nodeName, Name: nodeName, Instance: instance, Status: "online", Type: "node", LastSeen: now}},
		VMs: []models.VM{
			{ID: vmSourceID, VMID: 50110, Name: "www-DMZ", Node: nodeName, Instance: instance, Status: "running", Type: "qemu", OSName: "Debian", OSVersion: "12", NetworkInterfaces: []models.GuestNetworkInterface{{Name: "ens18", MAC: "02:00:00:00:00:10", Addresses: []string{"10.0.0.10"}}}, Disks: []models.Disk{{Device: "/dev/vda", Mountpoint: "/", Total: 1000, Used: 250}}, ConfigStorageVolumes: []models.ProxmoxVMStorageVolume{{DeviceKey: "scsi0", VolumeID: "local-zfs:vm-50110-disk-0", StorageID: "local-zfs"}}, ConfigObservedAt: now, ConfigObservationStatus: "current", LastSeen: now},
			{ID: siblingSource, VMID: 50111, Name: "alfheim", Node: nodeName, Instance: instance, Status: "running", Type: "qemu", LastSeen: now},
		},
		Storage: []models.Storage{{ID: instance + "-" + nodeName + "-local-zfs", Name: "local-zfs", Node: nodeName, Instance: instance, Type: "zfspool", Status: "available", Pool: "rpool", Active: true, LastSeen: now}},
		PhysicalDisks: []models.PhysicalDisk{
			{ID: instance + "-" + nodeName + "-disk-a", Node: nodeName, Instance: instance, DevPath: "/dev/sda", Model: "Samsung SSD", Serial: "SYNTH-DISK-A", Type: "sata", Size: 1000, Health: "PASSED", LastChecked: now},
			{ID: instance + "-" + nodeName + "-disk-b", Node: nodeName, Instance: instance, DevPath: "/dev/sdb", Model: "Samsung SSD", Serial: "SYNTH-DISK-B", Type: "sata", Size: 2000, Health: "PASSED", LastChecked: now},
		},
	}
	h := NewQueryService(&config.Config{DataPath: t.TempDir()})
	h.SetStateProvider(resourceStateProvider{snapshot: state})
	h.SetSupplementalRecordsProvider(unifiedresources.SourceKomodo, operatorRelationshipRecordProvider{records: []unifiedresources.IngestRecord{
		{SourceID: serverSourceID, Resource: unifiedresources.Resource{ID: serverSourceID, Name: "www-DMZ", Type: unifiedresources.ResourceTypeKomodoServer, Status: unifiedresources.StatusOnline, LastSeen: now}},
		{SourceID: siblingServerSourceID, Resource: unifiedresources.Resource{ID: siblingServerSourceID, Name: "alfheim", Type: unifiedresources.ResourceTypeKomodoServer, Status: unifiedresources.StatusOnline, LastSeen: now}},
		{SourceID: stackSourceID, ParentSourceID: serverSourceID, Resource: unifiedresources.Resource{ID: stackSourceID, Name: "kristi-www", Type: unifiedresources.ResourceTypeKomodoStack, Status: unifiedresources.StatusOnline, LastSeen: now, Relationships: []unifiedresources.ResourceRelationship{{SourceID: stackLinkID, TargetID: serverLinkID, Type: unifiedresources.RelHostedBy, Confidence: 1, EvidenceSource: unifiedresources.SourceKomodo, Active: true, Discoverer: "komodo_adapter", ObservedAt: now, LastSeenAt: now}}}},
		{SourceID: stack2SourceID, ParentSourceID: serverSourceID, Resource: unifiedresources.Resource{ID: stack2SourceID, Name: "metrics", Type: unifiedresources.ResourceTypeKomodoStack, Status: unifiedresources.StatusOnline, LastSeen: now, Relationships: []unifiedresources.ResourceRelationship{{SourceID: stack2LinkID, TargetID: serverLinkID, Type: unifiedresources.RelHostedBy, Confidence: 1, EvidenceSource: unifiedresources.SourceKomodo, Active: true, Discoverer: "komodo_adapter", ObservedAt: now, LastSeenAt: now}}}},
		{SourceID: siblingStackSourceID, ParentSourceID: siblingServerSourceID, Resource: unifiedresources.Resource{ID: siblingStackSourceID, Name: "alfheim-main", Type: unifiedresources.ResourceTypeKomodoStack, Status: unifiedresources.StatusOnline, LastSeen: now, Relationships: []unifiedresources.ResourceRelationship{{SourceID: siblingStackLinkID, TargetID: siblingServerLinkID, Type: unifiedresources.RelHostedBy, Confidence: 1, EvidenceSource: unifiedresources.SourceKomodo, Active: true, Discoverer: "komodo_adapter", ObservedAt: now, LastSeenAt: now}}}},
		{SourceID: siblingStack2SourceID, ParentSourceID: siblingServerSourceID, Resource: unifiedresources.Resource{ID: siblingStack2SourceID, Name: "alfheim-worker", Type: unifiedresources.ResourceTypeKomodoStack, Status: unifiedresources.StatusOnline, LastSeen: now, Relationships: []unifiedresources.ResourceRelationship{{SourceID: siblingStack2LinkID, TargetID: siblingServerLinkID, Type: unifiedresources.RelHostedBy, Confidence: 1, EvidenceSource: unifiedresources.SourceKomodo, Active: true, Discoverer: "komodo_adapter", ObservedAt: now, LastSeenAt: now}}}},
	}})
	h.SetSupplementalRecordsProvider(unifiedresources.SourceAvailability, availabilityRelationshipRecordProvider{records: []unifiedresources.IngestRecord{
		{SourceID: checkSourceID, Resource: unifiedresources.Resource{ID: checkSourceID, Name: "kristi-www HTTPS", Type: unifiedresources.ResourceTypeNetworkEndpoint, Status: unifiedresources.StatusOffline, LastSeen: now, Availability: &unifiedresources.AvailabilityData{TargetID: checkSourceID, LinkedResourceID: stackLinkID, Protocol: "https", Enabled: true, Available: false, LastChecked: &checkedAt}}},
		{SourceID: check2SourceID, Resource: unifiedresources.Resource{ID: check2SourceID, Name: "metrics HTTPS", Type: unifiedresources.ResourceTypeNetworkEndpoint, Status: unifiedresources.StatusOnline, LastSeen: now, Availability: &unifiedresources.AvailabilityData{TargetID: check2SourceID, LinkedResourceID: stack2LinkID, Protocol: "https", Enabled: true, Available: true, LastChecked: &checkedAt}}},
		{SourceID: siblingCheckSourceID, Resource: unifiedresources.Resource{ID: siblingCheckSourceID, Name: "alfheim-main HTTPS", Type: unifiedresources.ResourceTypeNetworkEndpoint, Status: unifiedresources.StatusOnline, LastSeen: now, Availability: &unifiedresources.AvailabilityData{TargetID: siblingCheckSourceID, LinkedResourceID: siblingStackLinkID, Protocol: "https", Enabled: true, Available: true, LastChecked: &checkedAt}}},
		{SourceID: siblingCheck2SourceID, Resource: unifiedresources.Resource{ID: siblingCheck2SourceID, Name: "alfheim-worker HTTPS", Type: unifiedresources.ResourceTypeNetworkEndpoint, Status: unifiedresources.StatusOnline, LastSeen: now, Availability: &unifiedresources.AvailabilityData{TargetID: siblingCheck2SourceID, LinkedResourceID: siblingStack2LinkID, Protocol: "https", Enabled: true, Available: true, LastChecked: &checkedAt}}},
	}})
	store, err := h.getStore(orgID)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := h.BuildRegistry(orgID)
	if err != nil {
		t.Fatal(err)
	}
	canonicalID := func(reference string) string {
		t.Helper()
		_, id, ok := registry.GetByReference(reference)
		if !ok {
			t.Fatalf("fixture reference %q did not resolve", reference)
		}
		return id
	}
	vmID := canonicalID(vmSourceID)
	serverID := canonicalID(serverSourceID)
	siblingID := canonicalID(siblingSource)
	siblingServerID := canonicalID(siblingServerSourceID)
	if err := store.AddLink(unifiedresources.ResourceLink{ResourceA: vmID, ResourceB: serverID, PrimaryID: vmID, Reason: "operator confirmed same machine", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddLink(unifiedresources.ResourceLink{ResourceA: siblingID, ResourceB: siblingServerID, PrimaryID: siblingID, Reason: "operator confirmed same machine", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	h.cacheMu.Lock()
	delete(h.registryCache, cacheKey(orgID))
	h.cacheMu.Unlock()
	registry, err = h.BuildRegistry(orgID)
	if err != nil {
		t.Fatal(err)
	}
	vmID = canonicalID(vmSourceID)
	siblingID = canonicalID(siblingSource)
	hostID := canonicalID(instance + "-" + nodeName)
	serverID = canonicalID(serverSourceID)
	if serverID != vmID {
		t.Fatalf("operator-confirmed Komodo Server identity resolved to %q, want canonical Proxmox VM %q", serverID, vmID)
	}
	stackID := canonicalID(stackSourceID)
	stack2ID := canonicalID(stack2SourceID)
	checkID := canonicalID(checkSourceID)
	check2ID := canonicalID(check2SourceID)
	disk1ID := canonicalID(instance + "-" + nodeName + "-disk-a")
	disk2ID := canonicalID(instance + "-" + nodeName + "-disk-b")
	storageID := canonicalID(instance + "-" + nodeName + "-local-zfs")
	request := httptest.NewRequest(http.MethodGet, "/api/resources/"+vmID, nil)
	request = request.WithContext(context.WithValue(request.Context(), apicontext.OrgIDContextKey, orgID))
	response := httptest.NewRecorder()
	h.HandleResourceRoutes(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET VM details status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		unifiedresources.Resource
		RelationshipContext struct {
			Resources []struct {
				ID       string   `json:"id"`
				Name     string   `json:"name"`
				ParentID string   `json:"parentId"`
				Distance int      `json:"distance"`
				Path     []string `json:"path"`
			} `json:"resources"`
			Relationships []unifiedresources.ResourceRelationship `json:"relationships"`
			Details       []struct {
				Relationship     unifiedresources.ResourceRelationship `json:"relationship"`
				Scope            string                                `json:"scope"`
				Classification   string                                `json:"classification"`
				Distance         int                                   `json:"distance"`
				TraversalPath    []string                              `json:"traversalPath"`
				ImpactAssessment string                                `json:"impactAssessment"`
			} `json:"relationshipDetails"`
		} `json:"relationshipContext"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.ID != vmID || payload.Proxmox == nil || payload.Proxmox.VMID != 50110 || payload.Proxmox.OSName != "Debian" || len(payload.Proxmox.Disks) != 1 || payload.Proxmox.Disks[0].Device != "/dev/vda" || len(payload.Proxmox.NetworkInterfaces) != 1 || payload.Proxmox.NetworkInterfaces[0].Name != "ens18" {
		var disk models.Disk
		if payload.Proxmox != nil && len(payload.Proxmox.Disks) > 0 {
			disk = models.Disk{Device: payload.Proxmox.Disks[0].Device, Mountpoint: payload.Proxmox.Disks[0].Mountpoint}
		}
		var vmid int
		var osName string
		var diskCount int
		if payload.Proxmox != nil {
			vmid, osName, diskCount = payload.Proxmox.VMID, payload.Proxmox.OSName, len(payload.Proxmox.Disks)
		}
		t.Fatalf("canonical VM observations = id:%s wantID:%s vmid:%d os:%q disks:%d first:%+v, want Proxmox VM identity, OS and guest-reported disk on the VM", payload.ID, vmID, vmid, osName, diskCount, disk)
	}
	if !containsDataSource(payload.Sources, unifiedresources.SourceKomodo) || !containsDataSource(payload.Sources, unifiedresources.SourceProxmox) {
		t.Fatalf("canonical VM providers = %v, want confirmed Komodo and Proxmox observations", payload.Sources)
	}
	for _, id := range []string{hostID, stackID, stack2ID, checkID, check2ID, siblingID, storageID, disk1ID, disk2ID} {
		found := false
		for _, resource := range payload.RelationshipContext.Resources {
			found = found || resource.ID == id
		}
		if !found {
			t.Fatalf("topology omitted %s from bounded context: %+v", id, payload.RelationshipContext.Resources)
		}
	}
	if len(payload.Relationships) != 4 {
		t.Fatalf("default resource relationship list = %+v, want VM hosting, verified storage backing and two directly hosted Stacks", payload.Relationships)
	}
	classifications := make(map[string]struct {
		scope            string
		classification   string
		distance         int
		path             []string
		impactAssessment string
	})
	for _, edge := range payload.RelationshipContext.Details {
		classifications[edge.Relationship.SourceID+"\x00"+edge.Relationship.TargetID] = struct {
			scope            string
			classification   string
			distance         int
			path             []string
			impactAssessment string
		}{edge.Scope, edge.Classification, edge.Distance, edge.TraversalPath, edge.ImpactAssessment}
	}
	assertEdge := func(sourceID, targetID, class string, distance int, path []string) unifiedresources.ResourceRelationship {
		t.Helper()
		got, ok := classifications[sourceID+"\x00"+targetID]
		if !ok {
			t.Fatalf("topology omitted classified edge %s -> %s: %+v", sourceID, targetID, payload.RelationshipContext.Details)
		}
		if got.classification != class || got.distance != distance || !reflect.DeepEqual(got.path, path) {
			t.Fatalf("classified edge %s -> %s = %+v, want class=%s distance=%d path=%v", sourceID, targetID, got, class, distance, path)
		}
		for _, edge := range payload.RelationshipContext.Relationships {
			if edge.SourceID == sourceID && edge.TargetID == targetID {
				return edge
			}
		}
		t.Fatalf("raw graph omitted edge %s -> %s", sourceID, targetID)
		return unifiedresources.ResourceRelationship{}
	}
	stackEdge := assertEdge(stackID, vmID, "direct_relationship", 1, []string{vmID, stackID})
	if stackEdge.Type != unifiedresources.RelHostedBy || stackEdge.EvidenceSource != unifiedresources.SourceKomodo || stackEdge.Discoverer != "komodo_adapter" {
		t.Fatalf("Stack hosting evidence = %+v", stackEdge)
	}
	backingEdge := assertEdge(vmID, storageID, "verified_hardware_backing", 1, []string{vmID, storageID})
	volumeEvidence, _ := backingEdge.Metadata["configVolumes"].([]any)
	var volume map[string]any
	if len(volumeEvidence) > 0 {
		volume, _ = volumeEvidence[0].(map[string]any)
	}
	if backingEdge.Type != unifiedresources.RelStoresOn || backingEdge.EvidenceSource != unifiedresources.SourceProxmox || backingEdge.Discoverer != "proxmox_vm_config" || volume["deviceKey"] != "scsi0" || volume["volumeId"] != "local-zfs:vm-50110-disk-0" {
		t.Fatalf("VM virtual disk backing evidence = %+v", backingEdge)
	}
	checkEdge := assertEdge(checkID, stackID, "service_monitoring", 2, []string{vmID, stackID, checkID})
	if checkEdge.Type != unifiedresources.RelChecks || checkEdge.EvidenceSource != unifiedresources.SourceAvailability {
		t.Fatalf("HTTPS monitoring edge = %+v", checkEdge)
	}
	diskEdge := assertEdge(disk1ID, hostID, "shared_infrastructure", 2, []string{vmID, hostID, disk1ID})
	if diskEdge.Type != unifiedresources.RelMountedTo || diskEdge.EvidenceSource != unifiedresources.SourceProxmox {
		t.Fatalf("host disk edge = %+v", diskEdge)
	}
	storageEdge := assertEdge(storageID, hostID, "shared_infrastructure", 2, []string{vmID, hostID, storageID})
	if storageEdge.Type != unifiedresources.RelMountedTo || storageEdge.EvidenceSource != unifiedresources.SourceProxmox {
		t.Fatalf("host storage edge = %+v", storageEdge)
	}
	siblingEdge := assertEdge(siblingID, hostID, "shared_infrastructure", 2, []string{vmID, hostID, siblingID})
	if siblingEdge.Type != unifiedresources.RelRunsOn || siblingEdge.EvidenceSource != unifiedresources.SourceProxmox {
		t.Fatalf("sibling placement edge = %+v", siblingEdge)
	}
	if _, exists := classifications[vmID+"\x00"+disk1ID]; exists {
		t.Fatal("guest-reported /dev/vda or shared host SSD was incorrectly promoted to VM backing")
	}
	if disk1ID == disk2ID {
		t.Fatal("same-name Samsung drives with distinct Proxmox native identities collapsed")
	}

	siblingRequest := httptest.NewRequest(http.MethodGet, "/api/resources/"+siblingID, nil)
	siblingRequest = siblingRequest.WithContext(context.WithValue(siblingRequest.Context(), apicontext.OrgIDContextKey, orgID))
	siblingResponse := httptest.NewRecorder()
	h.HandleResourceRoutes(siblingResponse, siblingRequest)
	if siblingResponse.Code != http.StatusOK {
		t.Fatalf("GET sibling VM details status=%d body=%s", siblingResponse.Code, siblingResponse.Body.String())
	}
	var siblingPayload struct {
		Relationships       []unifiedresources.ResourceRelationship `json:"relationships"`
		RelationshipContext struct {
			Details []struct {
				Relationship   unifiedresources.ResourceRelationship `json:"relationship"`
				Classification string                                `json:"classification"`
			} `json:"relationshipDetails"`
		} `json:"relationshipContext"`
	}
	if err := json.NewDecoder(siblingResponse.Body).Decode(&siblingPayload); err != nil {
		t.Fatal(err)
	}
	if len(siblingPayload.Relationships) != 3 {
		t.Fatalf("sibling VM direct relationships = %+v, want its host and two directly hosted Stacks", siblingPayload.Relationships)
	}
	stackHosting, checkCount := 0, 0
	for _, relationship := range siblingPayload.Relationships {
		if relationship.Type == unifiedresources.RelHostedBy && relationship.EvidenceSource == unifiedresources.SourceKomodo && relationship.Discoverer == "komodo_adapter" {
			stackHosting++
		}
	}
	for _, edge := range siblingPayload.RelationshipContext.Details {
		if edge.Relationship.Type == unifiedresources.RelChecks && edge.Classification == "service_monitoring" && edge.Relationship.EvidenceSource == unifiedresources.SourceAvailability {
			checkCount++
		}
	}
	if stackHosting != 2 || checkCount != 2 {
		t.Fatalf("sibling VM Stack and check associations = direct Komodo Stack edges %d, indirect availability checks %d; want two of each", stackHosting, checkCount)
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
