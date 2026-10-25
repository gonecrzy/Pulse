package unifiedresources

import (
	"errors"
	"testing"
	"time"
)

func TestSQLiteOperatorRelationshipsPersistAndIsolateOrganizations(t *testing.T) {
	dir := t.TempDir()
	orgA, err := NewSQLiteResourceStore(dir, "org-a")
	if err != nil {
		t.Fatal(err)
	}
	orgB, err := NewSQLiteResourceStore(dir, "org-b")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	edge := OperatorResourceRelationship{
		SourceID: "service:jellyfin", TargetID: "service:database", Type: RelDependsOn,
		Reason: "database is required", CreatedBy: "operator:alice", UpdatedBy: "operator:alice",
		CreatedAt: at, UpdatedAt: at,
	}
	if err := orgA.PutOperatorRelationship(edge); err != nil {
		t.Fatalf("PutOperatorRelationship(org-a): %v", err)
	}
	if err := orgA.Close(); err != nil {
		t.Fatal(err)
	}
	if err := orgB.Close(); err != nil {
		t.Fatal(err)
	}

	orgA, err = NewSQLiteResourceStore(dir, "org-a")
	if err != nil {
		t.Fatal(err)
	}
	defer orgA.Close()
	orgB, err = NewSQLiteResourceStore(dir, "org-b")
	if err != nil {
		t.Fatal(err)
	}
	defer orgB.Close()

	gotA, err := orgA.ListOperatorRelationships()
	if err != nil || len(gotA) != 1 {
		t.Fatalf("org-a relationships = %+v, err=%v; want one persisted edge", gotA, err)
	}
	if gotA[0].ID != OperatorRelationshipID(edge.SourceID, edge.TargetID, edge.Type) ||
		gotA[0].CreatedBy != "operator:alice" || gotA[0].Reason != edge.Reason || !gotA[0].CreatedAt.Equal(at) {
		t.Fatalf("persisted operator edge = %+v", gotA[0])
	}
	gotB, err := orgB.ListOperatorRelationships()
	if err != nil || len(gotB) != 0 {
		t.Fatalf("org-b relationships = %+v, err=%v; want none", gotB, err)
	}

	removedAt := at.Add(time.Hour)
	removed, err := orgA.RemoveOperatorRelationship(edge.SourceID, edge.TargetID, edge.Type, "operator:bob", removedAt)
	if err != nil || !removed {
		t.Fatalf("RemoveOperatorRelationship() = %t, %v; want true, nil", removed, err)
	}
	gotA, err = orgA.ListOperatorRelationships()
	if err != nil || len(gotA) != 0 {
		t.Fatalf("active org-a relationships after removal = %+v, err=%v; want none", gotA, err)
	}
	var removedBy string
	var storedRemovedAt time.Time
	if err := orgA.db.QueryRow(`SELECT removed_by, removed_at FROM operator_resource_relationships WHERE id = ?`, OperatorRelationshipID(edge.SourceID, edge.TargetID, edge.Type)).Scan(&removedBy, &storedRemovedAt); err != nil {
		t.Fatalf("read removal tombstone: %v", err)
	}
	if removedBy != "operator:bob" || !storedRemovedAt.Equal(removedAt) {
		t.Fatalf("removal tombstone = %s at %s", removedBy, storedRemovedAt)
	}
}

func TestOperatorRelationshipRejectsIdentityAndUnsupportedEdges(t *testing.T) {
	store := NewMemoryStore()
	for _, edge := range []OperatorResourceRelationship{
		{SourceID: "same", TargetID: "same", Type: RelDependsOn},
		{SourceID: "a", TargetID: "b", Type: RelationshipType("same_as")},
		{SourceID: "a", TargetID: "b", Type: RelChecks},
	} {
		if err := store.PutOperatorRelationship(edge); err == nil {
			t.Fatalf("PutOperatorRelationship(%+v) unexpectedly succeeded", edge)
		}
	}
}

func TestOperatorRelationshipsSurviveRebuildsAndRewriteLinkedEndpoints(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteResourceStore(dir, "org-a")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	pveSourceID := "pve:cluster-a:vm:101"
	komodoSourceID := "komodo:connection-a:server:a"
	dependencyID := "service:database"
	pveID := SourceSpecificID(ResourceTypeVM, SourceProxmox, pveSourceID)
	komodoID := SourceSpecificID(ResourceTypeKomodoServer, SourceKomodo, komodoSourceID)
	edge := OperatorResourceRelationship{
		SourceID: komodoID, TargetID: dependencyID, Type: RelDependsOn,
		Reason: "application data depends on database", CreatedBy: "operator:alice",
	}
	if err := store.PutOperatorRelationship(edge); err != nil {
		t.Fatal(err)
	}

	missing := NewRegistry(store)
	if got := missing.List(); len(got) != 0 {
		t.Fatalf("empty generation = %+v, want no phantom resources", got)
	}
	if rows, err := store.ListOperatorRelationships(); err != nil || len(rows) != 1 {
		t.Fatalf("edge after endpoint disappearance = %+v, err=%v", rows, err)
	}

	if err := store.AddLink(ResourceLink{ResourceA: pveID, ResourceB: komodoID, PrimaryID: pveID}); err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry(store)
	registry.IngestRecords(SourceProxmox, []IngestRecord{{
		SourceID: pveSourceID,
		Resource: Resource{Name: "Jellyfin host", Type: ResourceTypeVM, Status: StatusOnline, LastSeen: time.Now().UTC()},
	}})
	registry.IngestRecords(SourceKomodo, []IngestRecord{{
		SourceID: komodoSourceID,
		Resource: Resource{Name: "Jellyfin host", Type: ResourceTypeKomodoServer, Status: StatusOnline, LastSeen: time.Now().UTC()},
	}})
	resource, ok := registry.Get(pveID)
	if !ok {
		t.Fatalf("linked Proxmox canonical resource %q not found", pveID)
	}
	found := false
	for _, relationship := range resource.Relationships {
		if relationship.ID == OperatorRelationshipID(komodoID, dependencyID, RelDependsOn) {
			found = relationship.SourceID == pveID && relationship.TargetID == dependencyID &&
				relationship.Discoverer == operatorRelationshipDiscoverer && relationship.EvidenceFreshness == RelationshipFreshnessCurrent
		}
	}
	if !found {
		t.Fatalf("operator edge was not rewritten onto the linked canonical resource: %+v", resource.Relationships)
	}
	registry.MarkStale(time.Now().UTC().Add(24*time.Hour), nil)
	resource, ok = registry.Get(pveID)
	if !ok {
		t.Fatalf("linked resource disappeared after its providers went stale")
	}
	for _, relationship := range resource.Relationships {
		if relationship.ID == OperatorRelationshipID(komodoID, dependencyID, RelDependsOn) && relationship.EvidenceFreshness != RelationshipFreshnessCurrent {
			t.Fatalf("operator-authored edge became provider-stale: %+v", relationship)
		}
	}

	// A successful empty inventory makes the endpoint disappear from a new
	// registry generation, but does not delete operator intent from the store.
	removed := NewRegistry(store)
	if got := removed.List(); len(got) != 0 {
		t.Fatalf("removed generation = %+v, want no resource rows", got)
	}
	if rows, err := store.ListOperatorRelationships(); err != nil || len(rows) != 1 {
		t.Fatalf("stored edge after successful-empty rebuild = %+v, err=%v", rows, err)
	}
	returned := NewRegistry(store)
	returned.IngestRecords(SourceKomodo, []IngestRecord{{
		SourceID: komodoSourceID,
		Resource: Resource{Name: "Jellyfin host", Type: ResourceTypeKomodoServer, Status: StatusOnline, LastSeen: time.Now().UTC()},
	}})
	resource, ok = returned.Get(komodoID)
	if !ok || len(resource.Relationships) != 1 || resource.Relationships[0].TargetID != dependencyID {
		t.Fatalf("operator edge did not return with its endpoint: found=%t resource=%+v", ok, resource)
	}
}

func TestOperatorDependenciesCoexistWithAvailabilityCheckRelationship(t *testing.T) {
	store := NewMemoryStore()
	for _, target := range []string{"database", "cache"} {
		if err := store.PutOperatorRelationship(OperatorResourceRelationship{
			SourceID: "service:jellyfin", TargetID: "service:" + target, Type: RelDependsOn,
		}); err != nil {
			t.Fatal(err)
		}
	}
	registry := NewRegistry(store)
	registry.IngestResources([]Resource{{
		ID: "service:jellyfin", Type: ResourceTypeNetworkEndpoint, Name: "Jellyfin check",
		Relationships: []ResourceRelationship{{SourceID: "service:jellyfin", TargetID: "host:media", Type: RelChecks, Active: true, Discoverer: "availability_attachment"}},
	}})
	resource, ok := registry.Get("service:jellyfin")
	if !ok {
		t.Fatal("service resource not found")
	}
	counts := map[RelationshipType]int{}
	for _, relationship := range resource.Relationships {
		counts[relationship.Type]++
	}
	if counts[RelChecks] != 1 || counts[RelDependsOn] != 2 {
		t.Fatalf("check/dependency edges = %+v, want one checks and two depends_on", resource.Relationships)
	}
}

func TestDiscoveredRelationshipFreshnessFollowsProviderStatus(t *testing.T) {
	// IngestResources applies the registry's stale pass against the wall clock,
	// so keep fixture sightings relative to the test start time.
	observed := time.Now().UTC()
	resource := Resource{
		ID: "komodo-server-a", Type: ResourceTypeKomodoServer, Name: "server-a", Sources: []DataSource{SourceKomodo},
		Status:       StatusOnline,
		SourceStatus: map[DataSource]SourceStatus{SourceKomodo: {Status: "online", LastSeen: observed}},
		Relationships: []ResourceRelationship{{
			SourceID: "komodo-server-a", TargetID: "komodo-swarm-a", Type: RelMemberOf,
			Active: true, Discoverer: "komodo_adapter", EvidenceSource: SourceKomodo, LastSeenAt: observed,
		}},
	}
	registry := NewRegistryWithStaleThresholds(nil, map[DataSource]time.Duration{SourceKomodo: time.Minute})
	registry.IngestResources([]Resource{resource})
	registry.MarkStale(observed.Add(2*time.Minute), nil)
	got, ok := registry.Get(resource.ID)
	if !ok || got.Relationships[0].EvidenceFreshness != RelationshipFreshnessStale {
		t.Fatalf("stale discovered relationship = %+v, found=%t", got, ok)
	}

	resource.SourceStatus[SourceKomodo] = SourceStatus{Status: "online", LastSeen: observed.Add(3 * time.Minute)}
	resource.Relationships[0].LastSeenAt = observed.Add(3 * time.Minute)
	registry.IngestResources([]Resource{resource})
	got, ok = registry.Get(resource.ID)
	if !ok || got.Relationships[0].EvidenceFreshness != RelationshipFreshnessCurrent {
		t.Fatalf("recovered discovered relationship = %+v, found=%t", got, ok)
	}
}

func TestValidateIdentityLinkRejectsSameProviderCollisionAndAllowsCrossProvider(t *testing.T) {
	store := NewMemoryStore()
	proxmoxSource := "pve:cluster:vm:101"
	proxmoxSourceB := "pve:cluster:vm:102"
	komodoA := "komodo:connection:server:a"
	komodoB := "komodo:connection:server:b"
	pveID := SourceSpecificID(ResourceTypeVM, SourceProxmox, proxmoxSource)
	pveBID := SourceSpecificID(ResourceTypeVM, SourceProxmox, proxmoxSourceB)
	komodoAID := SourceSpecificID(ResourceTypeKomodoServer, SourceKomodo, komodoA)
	komodoBID := SourceSpecificID(ResourceTypeKomodoServer, SourceKomodo, komodoB)
	registry := NewRegistry(store)
	registry.IngestRecords(SourceProxmox, []IngestRecord{
		{SourceID: proxmoxSource, Resource: Resource{Name: "same", Type: ResourceTypeVM}},
		{SourceID: proxmoxSourceB, Resource: Resource{Name: "same", Type: ResourceTypeVM}},
	})
	registry.IngestRecords(SourceKomodo, []IngestRecord{
		{SourceID: komodoA, Resource: Resource{Name: "same", Type: ResourceTypeKomodoServer}},
		{SourceID: komodoB, Resource: Resource{Name: "same", Type: ResourceTypeKomodoServer}},
	})
	if err := registry.ValidateIdentityLink(pveID, komodoAID); err != nil {
		t.Fatalf("cross-provider Proxmox/Komodo link rejected: %v", err)
	}
	if err := store.AddLink(ResourceLink{ResourceA: pveID, ResourceB: komodoAID, PrimaryID: pveID}); err != nil {
		t.Fatal(err)
	}
	linked := NewRegistry(store)
	linked.IngestRecords(SourceProxmox, []IngestRecord{
		{SourceID: proxmoxSource, Resource: Resource{Name: "same", Type: ResourceTypeVM}},
		{SourceID: proxmoxSourceB, Resource: Resource{Name: "same", Type: ResourceTypeVM}},
	})
	linked.IngestRecords(SourceKomodo, []IngestRecord{
		{SourceID: komodoA, Resource: Resource{Name: "same", Type: ResourceTypeKomodoServer}},
		{SourceID: komodoB, Resource: Resource{Name: "same", Type: ResourceTypeKomodoServer}},
	})
	var conflict *IdentityLinkConflictError
	if err := linked.ValidateIdentityLink(pveID, komodoBID); err == nil || !errors.As(err, &conflict) {
		t.Fatalf("same-provider transitive link error = %v, want IdentityLinkConflictError", err)
	}
	if conflict.Source != SourceKomodo || len(conflict.SourceIDs) != 2 {
		t.Fatalf("conflict = %+v, want two Komodo source IDs", conflict)
	}
	var proxmoxConflict *IdentityLinkConflictError
	if err := linked.ValidateIdentityLink(pveBID, komodoAID); err == nil || !errors.As(err, &proxmoxConflict) {
		t.Fatalf("same-provider Proxmox link error = %v, want IdentityLinkConflictError", err)
	}
	if proxmoxConflict.Source != SourceProxmox || len(proxmoxConflict.SourceIDs) != 2 {
		t.Fatalf("Proxmox conflict = %+v, want two Proxmox source IDs", proxmoxConflict)
	}
	seeded := NewRegistry(store)
	seeded.IngestResources(linked.List())
	var rehydratedConflict *IdentityLinkConflictError
	if err := seeded.ValidateIdentityLink(komodoAID, komodoBID); err == nil || !errors.As(err, &rehydratedConflict) || rehydratedConflict.Source != SourceKomodo {
		t.Fatalf("in-memory unified snapshot lost Komodo source identity: conflict=%v", err)
	}
	var unavailable *IdentityLinkUnavailableError
	if err := linked.ValidateIdentityLink(pveID, "missing-resource"); err == nil || !errors.As(err, &unavailable) {
		t.Fatalf("missing-resource validation = %v, want IdentityLinkUnavailableError", err)
	}
}
