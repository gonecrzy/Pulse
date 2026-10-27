package unifiedresources

import (
	"testing"
	"time"

	"github.com/rcourtman/pulse-go-rewrite/internal/models"
)

func TestProxmoxVMConfigProjectsOnlyVerifiedStorageBacking(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	vm := Resource{
		ID: "vm-www-dmz", Type: ResourceTypeVM, Sources: []DataSource{SourceProxmox},
		Proxmox: &ProxmoxData{
			VMID: 50110, Instance: "cluster-a", NodeName: "pve01", VMConfigObservedAt: now,
			VMConfigStorageVolumes: []models.ProxmoxVMStorageVolume{
				{DeviceKey: "scsi0", VolumeID: "local-zfs:vm-50110-disk-0", StorageID: "local-zfs"},
				{DeviceKey: "scsi1", VolumeID: "local-zfs:vm-50110-disk-1", StorageID: "local-zfs"},
				{DeviceKey: "virtio0", VolumeID: "ceph-rbd:vm-50110-disk-1", StorageID: "ceph-rbd"},
			},
		},
		SourceStatus: map[DataSource]SourceStatus{SourceProxmox: {Status: "stale"}},
	}
	resources := []Resource{
		vm,
		{ID: "store-pve01", Type: ResourceTypeStorage, Name: "local-zfs", Sources: []DataSource{SourceProxmox},
			Proxmox: &ProxmoxData{Instance: "cluster-a", NodeName: "pve01"}, Storage: &StorageMeta{}},
		{ID: "store-pve02", Type: ResourceTypeStorage, Name: "local-zfs", Sources: []DataSource{SourceProxmox},
			Proxmox: &ProxmoxData{Instance: "cluster-a", NodeName: "pve02"}, Storage: &StorageMeta{}},
		{ID: "store-ceph", Type: ResourceTypeStorage, Name: "ceph-rbd", Sources: []DataSource{SourceProxmox},
			Proxmox: &ProxmoxData{Instance: "cluster-a", NodeName: "cluster"}, Storage: &StorageMeta{Shared: true, Nodes: []string{"pve01", "pve02"}}},
		{ID: "wrong-instance", Type: ResourceTypeStorage, Name: "local-zfs", Sources: []DataSource{SourceProxmox},
			Proxmox: &ProxmoxData{Instance: "cluster-b", NodeName: "pve01"}, Storage: &StorageMeta{}},
		{ID: "host-ssd", Type: ResourceTypePhysicalDisk, Name: "Samsung SSD", Sources: []DataSource{SourceProxmox},
			Proxmox: &ProxmoxData{Instance: "cluster-a", NodeName: "pve01"}},
	}

	got := ProxmoxVMStorageBackingRelationships(resources)
	if len(got) != 2 {
		t.Fatalf("verified VM storage relationships = %+v, want two config-backed stores_on edges", got)
	}
	wantTargets := map[string]bool{"store-pve01": true, "store-ceph": true}
	for _, relationship := range got {
		if relationship.SourceID != "vm-www-dmz" || relationship.Type != RelStoresOn || relationship.EvidenceSource != SourceProxmox || relationship.Discoverer != "proxmox_vm_config" {
			t.Errorf("relationship provenance or direction = %+v", relationship)
		}
		if !wantTargets[relationship.TargetID] {
			t.Errorf("unexpected backing target %q", relationship.TargetID)
		}
		delete(wantTargets, relationship.TargetID)
		if relationship.EvidenceFreshness != RelationshipFreshnessStale {
			t.Errorf("relationship freshness = %q, want stale", relationship.EvidenceFreshness)
		}
	}
	if len(wantTargets) != 0 {
		t.Errorf("missing verified storage targets %v", wantTargets)
	}
	for _, relationship := range got {
		if relationship.TargetID == "store-pve01" {
			volumes, ok := relationship.Metadata["configVolumes"].([]models.ProxmoxVMStorageVolume)
			if !ok || len(volumes) != 2 {
				t.Fatalf("same-store virtual disk evidence = %#v, want both device volume IDs on one canonical edge", relationship.Metadata["configVolumes"])
			}
		}
	}
	failedVM := vm
	failedVM.Proxmox = cloneProxmoxData(vm.Proxmox)
	failedVM.Proxmox.VMConfigObservationStatus = "unavailable"
	failedVM.SourceStatus = map[DataSource]SourceStatus{SourceProxmox: {Status: "online"}}
	failedResources := append([]Resource(nil), resources...)
	failedResources[0] = failedVM
	failedRelationships := ProxmoxVMStorageBackingRelationships(failedResources)
	if len(failedRelationships) != 2 || failedRelationships[0].EvidenceFreshness != RelationshipFreshnessStale {
		t.Fatalf("last-known backing evidence after failed config refresh = %+v, want both stale edges retained", failedRelationships)
	}
}

func TestDirectDependencyAndCrossBranchEdgesKeepDistinctClassifications(t *testing.T) {
	resources := map[string]Resource{
		"vm-www-dmz": {ID: "vm-www-dmz", Type: ResourceTypeVM},
		"proxy":      {ID: "proxy", Type: ResourceTypeAppContainer},
		"database":   {ID: "database", Type: ResourceTypeAppContainer},
	}
	route := ResourceRelationship{SourceID: "vm-www-dmz", TargetID: "proxy", Type: RelRoutesTo}
	if got := ClassifyResourceRelationshipContext("vm-www-dmz", route, []string{"vm-www-dmz", "proxy"}, resources); got != "cross_branch_relationship" {
		t.Fatalf("direct route classification = %q, want cross-branch relationship", got)
	}
	dependency := ResourceRelationship{SourceID: "vm-www-dmz", TargetID: "database", Type: RelDependsOn}
	if got := ClassifyResourceRelationshipContext("vm-www-dmz", dependency, []string{"vm-www-dmz", "database"}, resources); got != "direct_dependency" {
		t.Fatalf("direct dependency classification = %q, want direct dependency", got)
	}
}

func TestRelationshipTypeLabel(t *testing.T) {
	cases := map[RelationshipType]string{
		RelRunsOn:                       "Runs on",
		RelHostedBy:                     "Hosted by",
		RelDependsOn:                    "Depends on",
		RelRoutesTo:                     "Routes to",
		RelMountedTo:                    "Mounted to",
		RelExposedBy:                    "Exposed by",
		RelOwnedBy:                      "Owned by",
		RelAttachedTo:                   "Attached to",
		RelationshipType("custom_link"): "Custom link",
		RelationshipType(""):            "Related to",
	}

	for relationType, want := range cases {
		if got := RelationshipTypeLabel(relationType); got != want {
			t.Fatalf("RelationshipTypeLabel(%q) = %q, want %q", relationType, got, want)
		}
	}
}

func TestDescribeRelationship(t *testing.T) {
	presentation := DescribeRelationship(ResourceRelationship{
		SourceID:   "node-1",
		TargetID:   "vm-1",
		Type:       RelRunsOn,
		Confidence: 0.85,
		Active:     false,
		Discoverer: "docker_adapter",
		Metadata:   map[string]any{"region": "lab"},
	})

	if presentation.TypeLabel != "Runs on" {
		t.Fatalf("unexpected type label: %q", presentation.TypeLabel)
	}
	if presentation.Direction != "node-1 → vm-1" {
		t.Fatalf("unexpected direction: %q", presentation.Direction)
	}
	if presentation.Provenance != "docker_adapter" {
		t.Fatalf("unexpected provenance: %q", presentation.Provenance)
	}
	if presentation.StateLabel != "Historical" {
		t.Fatalf("unexpected state label: %q", presentation.StateLabel)
	}
	if presentation.Confidence != "85%" {
		t.Fatalf("unexpected confidence: %q", presentation.Confidence)
	}
	if !presentation.HasMetadata {
		t.Fatalf("expected metadata flag to be set")
	}
}

func TestResourceRelationshipSummary(t *testing.T) {
	summary := resourceRelationshipSummary([]ResourceRelationship{
		{
			SourceID: "node-1",
			TargetID: "vm-1",
			Type:     RelRunsOn,
		},
		{
			SourceID: "node-1",
			TargetID: "storage-1",
			Type:     RelDependsOn,
		},
	})

	if want := "node-1->storage-1[Depends on], node-1->vm-1[Runs on]"; summary != want {
		t.Fatalf("resourceRelationshipSummary() = %q, want %q", summary, want)
	}
}

func TestFormatResourceRelationshipContext(t *testing.T) {
	resource := &Resource{
		Relationships: []ResourceRelationship{
			{
				SourceID:   "node-1",
				TargetID:   "vm-1",
				Type:       RelRunsOn,
				Confidence: 0.85,
				Active:     true,
				Discoverer: "docker_adapter",
				Metadata:   map[string]any{"region": "lab"},
			},
			{
				SourceID:   "node-1",
				TargetID:   "storage-1",
				Type:       RelDependsOn,
				Confidence: 0.5,
				Active:     false,
			},
		},
	}

	ctx := FormatResourceRelationshipContext(resource, 1)
	if ctx == "" {
		t.Fatal("expected relationship context")
	}
	if want := "### Resource Relationships"; !contains(ctx, want) {
		t.Fatalf("expected %q in relationship context, got %q", want, ctx)
	}
	if !contains(ctx, "Runs on") {
		t.Fatalf("expected canonical relationship label, got %q", ctx)
	}
	if !contains(ctx, "discoverer docker_adapter") {
		t.Fatalf("expected provenance in relationship context, got %q", ctx)
	}
	if contains(ctx, "Depends on") {
		t.Fatalf("expected relationship limit to truncate entries, got %q", ctx)
	}
}

func TestFormatResourceRelationshipContextIncludesEvidenceSourceAndFreshness(t *testing.T) {
	context := FormatResourceRelationshipContext(&Resource{Relationships: []ResourceRelationship{{
		SourceID: "komodo-stack:web", TargetID: "komodo-server:edge", Type: RelHostedBy,
		Active: true, Discoverer: "komodo_adapter", EvidenceSource: SourceKomodo,
		EvidenceFreshness: RelationshipFreshnessStale,
	}}}, 5)
	for _, want := range []string{"evidence provider komodo", "evidence freshness stale"} {
		if !contains(context, want) {
			t.Fatalf("relationship context = %q, want %q", context, want)
		}
	}
}

func TestResourceRelationshipsWithCanonicalParent(t *testing.T) {
	parentID := "k8s-cluster-1"
	now := time.Date(2026, 3, 18, 17, 0, 0, 0, time.UTC)
	resource := Resource{
		ID:       "agent-1",
		Type:     ResourceTypeAgent,
		ParentID: &parentID,
		LastSeen: now,
	}

	relationships := ResourceRelationshipsWithCanonicalParent(resource)
	if len(relationships) != 1 {
		t.Fatalf("expected one canonical parent relationship, got %#v", relationships)
	}
	relationship := relationships[0]
	if relationship.SourceID != "agent-1" || relationship.TargetID != parentID {
		t.Fatalf("unexpected relationship endpoints: %#v", relationship)
	}
	if relationship.Type != RelOwnedBy {
		t.Fatalf("relationship type = %q, want %q", relationship.Type, RelOwnedBy)
	}
	if relationship.Discoverer != parentRelationshipDiscoverer {
		t.Fatalf("discoverer = %q, want %q", relationship.Discoverer, parentRelationshipDiscoverer)
	}
	if relationship.Metadata["source"] != "parentId" {
		t.Fatalf("expected parentId provenance metadata, got %#v", relationship.Metadata)
	}

	relationships = ResourceRelationshipsWithCanonicalParent(Resource{
		ID:       "agent-1",
		Type:     ResourceTypeAgent,
		ParentID: &parentID,
		Relationships: []ResourceRelationship{
			{
				SourceID: "agent-1",
				TargetID: parentID,
				Type:     RelOwnedBy,
			},
		},
	})
	if len(relationships) != 1 {
		t.Fatalf("expected existing canonical parent relationship to be reused, got %#v", relationships)
	}
}

func TestCanonicalParentFreshnessUsesItsProviderAfterIdentityFold(t *testing.T) {
	parentID := "host:media"
	resource := Resource{
		ID: "service:jellyfin", Type: ResourceTypeKomodoDeployment, ParentID: &parentID,
		Sources: []DataSource{SourceKomodo, SourceProxmox},
		SourceStatus: map[DataSource]SourceStatus{
			SourceKomodo: {Status: "stale"}, SourceProxmox: {Status: "online"},
		},
		parentBySource: map[DataSource]string{SourceKomodo: parentID},
	}
	relationships := ResourceRelationshipsWithCanonicalParent(resource)
	if len(relationships) != 1 {
		t.Fatalf("parent relationships = %+v, want one", relationships)
	}
	if got := relationships[0]; got.EvidenceSource != SourceKomodo || got.EvidenceFreshness != RelationshipFreshnessStale {
		t.Fatalf("parent evidence = %+v, want stale Komodo evidence", got)
	}
}

func TestNormalizeResourceRelationshipAddsStableAuditIdentity(t *testing.T) {
	relationship := ResourceRelationship{
		SourceID:   " network-endpoint:check-1 ",
		TargetID:   " app-container:web ",
		Type:       RelChecks,
		Confidence: 1,
		Discoverer: " availability_attachment ",
		EvidenceID: " evidence-1 ",
	}
	normalizeResourceRelationship(&relationship)
	firstID := relationship.ID

	if firstID == "" {
		t.Fatal("relationship ID was not derived")
	}
	if relationship.SourceID != "network-endpoint:check-1" ||
		relationship.TargetID != "app-container:web" {
		t.Fatalf("canonical endpoints = %q -> %q", relationship.SourceID, relationship.TargetID)
	}
	if relationship.EvidenceID != "evidence-1" {
		t.Fatalf("evidence ID = %q", relationship.EvidenceID)
	}

	relationship.ID = ""
	normalizeResourceRelationship(&relationship)
	if relationship.ID != firstID {
		t.Fatalf("relationship ID changed: got %q want %q", relationship.ID, firstID)
	}
}

func TestCanonicalRelationshipPresentationDeduplicatesByOwnerAndPreservesParallelEvidence(t *testing.T) {
	base := ResourceRelationship{
		SourceID: " vm:www ", TargetID: " komodo-server:www ", Type: RelHostedBy,
		Discoverer: "komodo_adapter", EvidenceSource: SourceKomodo, EvidenceID: "stack-server-map",
		Active: true, LastSeenAt: time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC),
	}
	duplicate := base
	duplicate.SourceID = "vm:www"
	duplicate.TargetID = "komodo-server:www"
	duplicate.LastSeenAt = base.LastSeenAt.Add(time.Minute)
	providerParallel := base
	providerParallel.EvidenceSource = SourceProxmox
	manualParallel := base
	manualParallel.Discoverer = "operator"
	manualParallel.CreatedBy = "operator@example.test"

	merged := mergeResourceRelationships([]ResourceRelationship{base}, []ResourceRelationship{duplicate, providerParallel, manualParallel})
	if len(merged) != 3 {
		t.Fatalf("canonical relationships = %+v, want one deduplicated Komodo edge plus distinct Proxmox and manual edges", merged)
	}
	owners := make(map[string]ResourceRelationship, len(merged))
	ids := make(map[string]struct{}, len(merged))
	for _, relationship := range merged {
		owners[RelationshipPresentationKey(relationship)] = relationship
		if _, exists := ids[relationship.ID]; exists {
			t.Fatalf("parallel relationship ID %q collided: %+v", relationship.ID, merged)
		}
		ids[relationship.ID] = struct{}{}
	}
	if got := owners[RelationshipPresentationKey(base)]; got.LastSeenAt != duplicate.LastSeenAt {
		t.Fatalf("same-owner duplicate = %+v, want newest observation at %s", got, duplicate.LastSeenAt)
	}
	if _, ok := owners[RelationshipPresentationKey(providerParallel)]; !ok {
		t.Fatal("distinct Proxmox provider observation was discarded")
	}
	if _, ok := owners[RelationshipPresentationKey(manualParallel)]; !ok {
		t.Fatal("distinct manual relationship was discarded")
	}
}

func TestOperationalTrustRelationshipVocabularyIsTyped(t *testing.T) {
	for _, relationshipType := range []RelationshipType{
		RelRunsOn,
		RelHostedBy,
		RelDependsOn,
		RelStoresOn,
		RelProtectedBy,
		RelChecks,
		RelMemberOf,
		RelRoutesTo,
		RelExposedBy,
	} {
		if relationshipType == "" {
			t.Fatal("relationship type must not be empty")
		}
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
