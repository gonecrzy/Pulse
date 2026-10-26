package komodo

import (
	"testing"
	"time"

	"github.com/rcourtman/pulse-go-rewrite/internal/models"
	"github.com/rcourtman/pulse-go-rewrite/internal/unifiedresources"
)

func TestProxmoxKomodoLinkPersistsAcrossStoreRestartAndKeepsAssociations(t *testing.T) {
	const (
		orgID        = "org-komodo-identity-test"
		connectionID = "87ef53c3-9a5a-4d7b-a159-997ccca19811"
		pveSourceID  = "pve:cluster-a:vmid:104"
		serverID     = "server-jellyfin"
		swarmID      = "swarm-production"
		probeID      = "probe-jellyfin"
	)
	dataDir := t.TempDir()
	pveID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeVM, unifiedresources.SourceProxmox, pveSourceID)
	komodoServerSourceID := SourceID(connectionID, KindServer, serverID)
	komodoID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeKomodoServer, unifiedresources.SourceKomodo, komodoServerSourceID)
	swarmSourceID := SourceID(connectionID, KindSwarm, swarmID)
	swarmCanonicalID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeKomodoSwarm, unifiedresources.SourceKomodo, swarmSourceID)
	now := time.Now().UTC()
	pveRecords, komodoRecords, availabilityRecords := linkedEstateRecords(connectionID, pveSourceID, serverID, swarmID, probeID, now)

	store, err := unifiedresources.NewSQLiteResourceStore(dataDir, orgID)
	if err != nil {
		t.Fatalf("open resource store: %v", err)
	}
	if err := store.AddLink(unifiedresources.ResourceLink{
		ResourceA: pveID,
		ResourceB: komodoID,
		PrimaryID: pveID,
		Reason:    "operator-confirmed same VM",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("persist operator identity link: %v", err)
	}

	// The incremental poll order starts with Komodo and sees Proxmox later.
	registry := unifiedresources.NewRegistry(store)
	registry.IngestRecords(unifiedresources.SourceKomodo, komodoRecords)
	registry.IngestRecords(unifiedresources.SourceProxmox, pveRecords)
	if resolved, ok := registry.ResolveReferenceID(komodoID); !ok || resolved != pveID {
		t.Fatalf("Komodo member reference resolves to %q, %t, want canonical Proxmox VM %q", resolved, ok, pveID)
	}
	registry.IngestRecords(unifiedresources.SourceAvailability, availabilityRecords)
	assertLinkedEstate(t, registry, pveID, komodoID, komodoServerSourceID, pveSourceID, swarmCanonicalID, probeID)
	if err := store.Close(); err != nil {
		t.Fatalf("close resource store: %v", err)
	}

	// A new store and registry generation must reload the operator decision.
	reopened, err := unifiedresources.NewSQLiteResourceStore(dataDir, orgID)
	if err != nil {
		t.Fatalf("reopen resource store: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	rebuilt := unifiedresources.NewRegistry(reopened)
	rebuilt.IngestRecords(unifiedresources.SourceProxmox, pveRecords)
	rebuilt.IngestRecords(unifiedresources.SourceKomodo, komodoRecords)
	// Reconciliation processes availability records after infrastructure
	// sources so their explicit links resolve against the completed graph.
	rebuilt.IngestRecords(unifiedresources.SourceAvailability, availabilityRecords)
	assertLinkedEstate(t, rebuilt, pveID, komodoID, komodoServerSourceID, pveSourceID, swarmCanonicalID, probeID)

	// The same link is organization-scoped. A second org with identical native
	// identities but no stored decision keeps its two host rows separate.
	otherStore, err := unifiedresources.NewSQLiteResourceStore(dataDir, "org-komodo-identity-other")
	if err != nil {
		t.Fatalf("open second organization store: %v", err)
	}
	t.Cleanup(func() { _ = otherStore.Close() })
	other := unifiedresources.NewRegistry(otherStore)
	other.IngestRecords(unifiedresources.SourceProxmox, pveRecords)
	other.IngestRecords(unifiedresources.SourceKomodo, komodoRecords)
	if resources := other.List(); len(resources) != 4 {
		t.Fatalf("unlinked org inventory has %d resources, want separate Proxmox VM, Komodo Server, Swarm, and Stack", len(resources))
	}
	if pve, ok := other.Get(pveID); !ok {
		t.Fatalf("second organization lost Proxmox resource %q", pveID)
	} else if hasSource(pve.Sources, unifiedresources.SourceKomodo) {
		t.Fatalf("second organization merged resources without its own link: %+v", pve)
	}
}

func TestWWWDMZStackAvailabilityChainPersistsAcrossRestart(t *testing.T) {
	const (
		orgID        = "org-www-dmz-topology-test"
		connectionID = "87ef53c3-9a5a-4d7b-a159-997ccca19811"
		pveSourceID  = "pve:cluster-a:vmid:50110"
		serverID     = "server-www-dmz"
		stackID      = "stack-kristi-www"
		probeID      = "kristi-www-site"
	)
	now := time.Now().UTC()
	pveID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeVM, unifiedresources.SourceProxmox, pveSourceID)
	serverSourceID := SourceID(connectionID, KindServer, serverID)
	serverCanonicalID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeKomodoServer, unifiedresources.SourceKomodo, serverSourceID)
	stackSourceID := SourceID(connectionID, KindStack, stackID)
	stackCanonicalID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeKomodoStack, unifiedresources.SourceKomodo, stackSourceID)
	checkCanonicalID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeNetworkEndpoint, unifiedresources.SourceAvailability, probeID)

	pveRecords := []unifiedresources.IngestRecord{{
		SourceID: pveSourceID,
		Resource: unifiedresources.Resource{
			Type: unifiedresources.ResourceTypeVM, Name: "www-DMZ", Status: unifiedresources.StatusOnline,
			LastSeen: now, Proxmox: &unifiedresources.ProxmoxData{VMID: 50110, Instance: "cluster-a"},
		},
	}}
	komodoRecords := NormalizeInventory(connectionID, Inventory{
		Servers: []Resource{{ID: serverID, Name: "www-DMZ", Kind: KindServer, State: "Ok"}},
		Stacks:  []Resource{{ID: stackID, Name: "kristi-www", Kind: KindStack, State: "Running", ServerID: serverID}},
	}, now)
	availabilityRecords := []unifiedresources.IngestRecord{{
		SourceID: probeID,
		Resource: unifiedresources.Resource{
			Type: unifiedresources.ResourceTypeNetworkEndpoint, Name: probeID, Status: unifiedresources.StatusOnline,
			LastSeen: now, Sources: []unifiedresources.DataSource{unifiedresources.SourceAvailability},
			Availability: &unifiedresources.AvailabilityData{
				TargetID: probeID, LinkedResourceID: stackCanonicalID, Address: "192.0.2.110", Protocol: "https", Enabled: true, Available: true,
			},
		},
	}}

	dataDir := t.TempDir()
	store, err := unifiedresources.NewSQLiteResourceStore(dataDir, orgID)
	if err != nil {
		t.Fatalf("open resource store: %v", err)
	}
	if err := store.AddLink(unifiedresources.ResourceLink{
		ResourceA: pveID, ResourceB: serverCanonicalID, PrimaryID: pveID,
		Reason: "operator-confirmed same VM", CreatedAt: now,
	}); err != nil {
		t.Fatalf("persist operator identity link: %v", err)
	}

	assertChain := func(registry *unifiedresources.ResourceRegistry) {
		t.Helper()
		resources := registry.List()
		if len(resources) != 3 {
			t.Fatalf("canonical inventory has %d resources, want one host, one Stack, and one check: %+v", len(resources), resources)
		}
		host, ok := registry.Get(pveID)
		if !ok || host.ID != pveID || host.Type != unifiedresources.ResourceTypeVM {
			t.Fatalf("canonical host = %+v, want the Proxmox VM identity %q", host, pveID)
		}
		if !hasSource(host.Sources, unifiedresources.SourceProxmox) || !hasSource(host.Sources, unifiedresources.SourceKomodo) {
			t.Fatalf("canonical host sources = %v, want Proxmox and Komodo", host.Sources)
		}
		if target := registry.MetricsTarget(pveID); target == nil || target.ResourceType != "vm" || target.ResourceID != pveSourceID {
			t.Fatalf("metrics target = %+v, want vm/%s", target, pveSourceID)
		}
		if host.Proxmox == nil || host.Proxmox.VMID != 50110 {
			t.Fatalf("Proxmox identity facet = %+v, want VMID 50110", host.Proxmox)
		}
		stack, ok := registry.Get(stackCanonicalID)
		if !ok || stack.ParentID == nil || *stack.ParentID != pveID {
			t.Fatalf("Stack = %+v, want hosted_by canonical host %q", stack, pveID)
		}
		if stack.Canonical == nil || stack.Canonical.PrimaryID != stackCanonicalID {
			t.Fatalf("Stack canonical identity = %+v, want native Komodo identity %q", stack.Canonical, stackCanonicalID)
		}
		for _, alias := range stack.Canonical.Aliases {
			if alias == probeID || alias == "192.0.2.110" {
				t.Fatalf("Stack identity borrowed availability alias %q: %+v", alias, stack.Canonical)
			}
		}
		stackEdges := unifiedresources.ResourceRelationshipsWithCanonicalParent(*stack)
		if len(stackEdges) != 1 || stackEdges[0].Type != unifiedresources.RelHostedBy || stackEdges[0].TargetID != pveID || stackEdges[0].EvidenceSource != unifiedresources.SourceKomodo {
			t.Fatalf("Stack relationships = %+v, want Komodo hosted_by canonical host", stackEdges)
		}
		check, ok := registry.Get(checkCanonicalID)
		if !ok || check.Type != unifiedresources.ResourceTypeNetworkEndpoint || check.ID != checkCanonicalID {
			t.Fatalf("availability check = %+v, want its independent endpoint identity %q", check, checkCanonicalID)
		}
		if check.Availability == nil || check.Availability.ResolvedResourceID != stackCanonicalID || check.Availability.LinkedResourceName != "kristi-www" {
			t.Fatalf("resolved check association = %+v, want canonical Stack identity and label", check.Availability)
		}
		checkEdges := unifiedresources.ResourceRelationshipsWithCanonicalParent(*check)
		if len(checkEdges) != 1 || checkEdges[0].Type != unifiedresources.RelChecks || checkEdges[0].TargetID != stackCanonicalID {
			t.Fatalf("availability relationships = %+v, want checks edge to Stack %q", checkEdges, stackCanonicalID)
		}
		checks := unifiedresources.AvailabilityChecksForResource(*stack)
		if len(checks) != 1 || checks[0].TargetID != probeID || checks[0].LinkedResourceID != stackCanonicalID {
			t.Fatalf("Stack availability checks = %+v, want %q attached without replacing Stack identity", checks, probeID)
		}
	}

	registry := unifiedresources.NewRegistry(store)
	registry.IngestRecords(unifiedresources.SourceKomodo, komodoRecords)
	registry.IngestRecords(unifiedresources.SourceProxmox, pveRecords)
	registry.IngestRecords(unifiedresources.SourceAvailability, availabilityRecords)
	assertChain(registry)
	if err := store.Close(); err != nil {
		t.Fatalf("close resource store: %v", err)
	}

	reopened, err := unifiedresources.NewSQLiteResourceStore(dataDir, orgID)
	if err != nil {
		t.Fatalf("reopen resource store: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	rebuilt := unifiedresources.NewRegistry(reopened)
	rebuilt.IngestRecords(unifiedresources.SourceProxmox, pveRecords)
	rebuilt.IngestRecords(unifiedresources.SourceKomodo, komodoRecords)
	rebuilt.IngestRecords(unifiedresources.SourceAvailability, availabilityRecords)
	assertChain(rebuilt)
}

func TestProxmoxKomodoLinkStalenessRemovalAndRecovery(t *testing.T) {
	const (
		connectionID = "87ef53c3-9a5a-4d7b-a159-997ccca19811"
		pveSourceID  = "pve:cluster-a:vmid:104"
		serverID     = "server-jellyfin"
	)
	now := time.Now().UTC()
	old := now.Add(-10 * time.Minute)
	pveID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeVM, unifiedresources.SourceProxmox, pveSourceID)
	komodoSourceID := SourceID(connectionID, KindServer, serverID)
	komodoID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeKomodoServer, unifiedresources.SourceKomodo, komodoSourceID)
	store := unifiedresources.NewMemoryStore()
	if err := store.AddLink(unifiedresources.ResourceLink{ResourceA: pveID, ResourceB: komodoID, PrimaryID: pveID}); err != nil {
		t.Fatalf("persist manual link: %v", err)
	}
	thresholds := map[unifiedresources.DataSource]time.Duration{
		unifiedresources.SourceProxmox: time.Minute,
		unifiedresources.SourceKomodo:  time.Minute,
	}

	for _, staleSource := range []unifiedresources.DataSource{unifiedresources.SourceKomodo, unifiedresources.SourceProxmox} {
		t.Run(string(staleSource)+"_stale", func(t *testing.T) {
			pveAt, komodoAt := now, now
			if staleSource == unifiedresources.SourceProxmox {
				pveAt = old
			} else {
				komodoAt = old
			}
			pve, komodo, _ := linkedEstateRecords(connectionID, pveSourceID, serverID, "swarm-production", "probe-jellyfin", now)
			pve[0].Resource.LastSeen = pveAt
			komodo = NormalizeInventory(connectionID, Inventory{
				Servers: []Resource{{ID: serverID, Name: "Jellyfin-DMZ", Kind: KindServer, State: "Ok"}},
			}, komodoAt)
			registry := unifiedresources.NewRegistryWithStaleThresholds(store, thresholds)
			registry.IngestRecords(unifiedresources.SourceKomodo, komodo)
			registry.IngestRecords(unifiedresources.SourceProxmox, pve)
			registry.MarkStale(now, thresholds)
			resources := registry.List()
			if len(resources) != 1 || resources[0].ID != pveID || resources[0].Type != unifiedresources.ResourceTypeVM {
				t.Fatalf("canonical resources with %s stale = %+v, want the selected Proxmox VM identity", staleSource, resources)
			}
			if got := resources[0].SourceStatus[staleSource].Status; got != "stale" {
				t.Fatalf("%s source status = %q, want stale", staleSource, got)
			}
		})
	}

	// A full supplemental snapshot, unlike an incremental IngestRecords call,
	// replaces the prior source inventory. A successful empty Komodo snapshot
	// removes the secondary record and leaves the primary Proxmox identity
	// intact; the stored decision applies again on recovery.
	pve, komodo, _ := linkedEstateRecords(connectionID, pveSourceID, serverID, "swarm-production", "probe-jellyfin", now)
	adapter := unifiedresources.NewMonitorAdapter(unifiedresources.NewRegistry(store))
	adapter.PopulateSnapshotAndSupplemental(models.StateSnapshot{}, map[unifiedresources.DataSource][]unifiedresources.IngestRecord{
		unifiedresources.SourceProxmox: pve,
		unifiedresources.SourceKomodo:  komodo,
	})
	adapter.PopulateSnapshotAndSupplemental(models.StateSnapshot{}, map[unifiedresources.DataSource][]unifiedresources.IngestRecord{
		unifiedresources.SourceProxmox: pve,
		unifiedresources.SourceKomodo:  nil,
	})
	if resources := adapter.GetAll(); len(resources) != 1 || resources[0].ID != pveID {
		t.Fatalf("inventory after successful empty Komodo snapshot = %+v, want the Proxmox primary to remain", resources)
	}
	adapter.PopulateSnapshotAndSupplemental(models.StateSnapshot{}, map[unifiedresources.DataSource][]unifiedresources.IngestRecord{
		unifiedresources.SourceProxmox: pve,
		unifiedresources.SourceKomodo:  komodo,
	})
	if resources := adapter.GetAll(); len(resources) != 3 {
		// The restored Server folds into the Proxmox VM; Swarm and Stack remain
		// separate resources with their parent relationships intact.
		t.Fatalf("inventory after Komodo recovery = %+v, want three rows with the VM/Server pair folded", resources)
	}
	if _, _, ok := adapter.GetByReference(pveID); !ok {
		t.Fatalf("Proxmox primary %q did not return after Komodo recovery", pveID)
	}

	// If the selected primary provider is absent, the visible surviving source
	// remains usable under its own ID. The persisted link is reapplied if the
	// primary source returns; absence alone does not create a synthetic match.
	adapter.PopulateSnapshotAndSupplemental(models.StateSnapshot{}, map[unifiedresources.DataSource][]unifiedresources.IngestRecord{
		unifiedresources.SourceKomodo:  komodo[:1],
		unifiedresources.SourceProxmox: pve,
	})
	adapter.PopulateSnapshotAndSupplemental(models.StateSnapshot{}, map[unifiedresources.DataSource][]unifiedresources.IngestRecord{
		unifiedresources.SourceKomodo:  komodo[:1],
		unifiedresources.SourceProxmox: nil,
	})
	if resources := adapter.GetAll(); len(resources) != 1 || resources[0].ID != komodoID {
		t.Fatalf("inventory after successful empty Proxmox snapshot = %+v, want the live Komodo Server under its own ID", resources)
	}
	adapter.PopulateSnapshotAndSupplemental(models.StateSnapshot{}, map[unifiedresources.DataSource][]unifiedresources.IngestRecord{
		unifiedresources.SourceKomodo:  komodo[:1],
		unifiedresources.SourceProxmox: pve,
	})
	if resources := adapter.GetAll(); len(resources) != 1 || resources[0].ID != pveID {
		t.Fatalf("inventory after Proxmox recovery = %+v, want the stored primary ID restored", resources)
	}
}

func TestSameNamedKomodoResourcesOnlyFoldToTheOperatorSelectedProxmoxIdentity(t *testing.T) {
	const (
		connectionA = "87ef53c3-9a5a-4d7b-a159-997ccca19811"
		connectionB = "87ef53c3-9a5a-4d7b-a159-997ccca19812"
	)
	pveA := "pve:cluster-a:vmid:104"
	pveB := "pve:cluster-a:vmid:105"
	serverNativeID := "server-jellyfin"
	pveAID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeVM, unifiedresources.SourceProxmox, pveA)
	pveBID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeVM, unifiedresources.SourceProxmox, pveB)
	komodoASourceID := SourceID(connectionA, KindServer, serverNativeID)
	komodoBSourceID := SourceID(connectionB, KindServer, serverNativeID)
	komodoAID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeKomodoServer, unifiedresources.SourceKomodo, komodoASourceID)
	komodoBID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeKomodoServer, unifiedresources.SourceKomodo, komodoBSourceID)
	registry := unifiedresources.NewRegistryWithManualLinks([]unifiedresources.ResourceLink{{
		ResourceA: pveAID, ResourceB: komodoAID, PrimaryID: pveAID,
		Reason: "operator-selected exact identities", CreatedAt: time.Now().UTC(),
	}})
	registry.IngestRecords(unifiedresources.SourceKomodo, append(
		NormalizeInventory(connectionA, Inventory{Servers: []Resource{{ID: serverNativeID, Name: "Jellyfin-DMZ", Kind: KindServer, State: "Ok"}}}, time.Now().UTC()),
		NormalizeInventory(connectionB, Inventory{Servers: []Resource{{ID: serverNativeID, Name: "Jellyfin-DMZ", Kind: KindServer, State: "Ok"}}}, time.Now().UTC())...,
	))
	registry.IngestRecords(unifiedresources.SourceProxmox, []unifiedresources.IngestRecord{
		{SourceID: pveA, Resource: unifiedresources.Resource{Type: unifiedresources.ResourceTypeVM, Name: "Jellyfin-DMZ", Status: unifiedresources.StatusOnline}},
		{SourceID: pveB, Resource: unifiedresources.Resource{Type: unifiedresources.ResourceTypeVM, Name: "Jellyfin-DMZ", Status: unifiedresources.StatusOnline}},
	})

	resources := registry.List()
	if len(resources) != 3 {
		t.Fatalf("same-named ambiguous inventory = %+v, want only the selected PVE/Komodo pair folded", resources)
	}
	if pve, ok := registry.Get(pveAID); !ok || !hasSource(pve.Sources, unifiedresources.SourceKomodo) {
		t.Fatalf("operator-selected pair did not fold: %+v", pve)
	}
	if pve, ok := registry.Get(pveBID); !ok || hasSource(pve.Sources, unifiedresources.SourceKomodo) {
		t.Fatalf("unlinked same-named Proxmox VM was merged: %+v", pve)
	}
	if komodo, ok := registry.Get(komodoBID); !ok || hasSource(komodo.Sources, unifiedresources.SourceProxmox) {
		t.Fatalf("same-named Komodo resource from another connection was merged: %+v", komodo)
	}
}

func TestConflictingProxmoxKomodoIdentityLinksFormDeterministicExplicitEquivalence(t *testing.T) {
	const connectionID = "87ef53c3-9a5a-4d7b-a159-997ccca19811"
	const serverNativeID = "server-jellyfin"
	komodoSourceID := SourceID(connectionID, KindServer, serverNativeID)
	komodoID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeKomodoServer, unifiedresources.SourceKomodo, komodoSourceID)
	pveSourceIDs := []string{"pve:cluster-a:vmid:104", "pve:cluster-a:vmid:105"}
	pveIDs := []string{
		unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeVM, unifiedresources.SourceProxmox, pveSourceIDs[0]),
		unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeVM, unifiedresources.SourceProxmox, pveSourceIDs[1]),
	}
	created := time.Unix(500, 0).UTC()
	links := []unifiedresources.ResourceLink{
		{ResourceA: pveIDs[0], ResourceB: komodoID, PrimaryID: pveIDs[0], CreatedAt: created},
		{ResourceA: pveIDs[1], ResourceB: komodoID, PrimaryID: pveIDs[1], CreatedAt: created.Add(time.Minute)},
	}
	komodoRecords := NormalizeInventory(connectionID, Inventory{
		Servers: []Resource{{ID: serverNativeID, Name: "Jellyfin-DMZ", Kind: KindServer, State: "Ok"}},
	}, created.Add(2*time.Minute))
	pveRecords := []unifiedresources.IngestRecord{
		{SourceID: pveSourceIDs[0], Resource: unifiedresources.Resource{Type: unifiedresources.ResourceTypeVM, Name: "Jellyfin-DMZ", Status: unifiedresources.StatusOnline}},
		{SourceID: pveSourceIDs[1], Resource: unifiedresources.Resource{Type: unifiedresources.ResourceTypeVM, Name: "Jellyfin-DMZ", Status: unifiedresources.StatusOnline}},
	}

	var firstCanonical string
	for index, linkOrder := range [][]unifiedresources.ResourceLink{links, {links[1], links[0]}} {
		registry := unifiedresources.NewRegistryWithManualLinks(linkOrder)
		registry.IngestRecords(unifiedresources.SourceKomodo, komodoRecords)
		registry.IngestRecords(unifiedresources.SourceProxmox, pveRecords)

		resources := registry.List()
		var canonicalWithKomodo string
		for _, resource := range resources {
			if hasSource(resource.Sources, unifiedresources.SourceKomodo) {
				if canonicalWithKomodo != "" {
					t.Fatalf("link order %d folded one Komodo identity into multiple resources: %+v", index, resources)
				}
				canonicalWithKomodo = resource.ID
			}
		}
		if len(resources) != 1 {
			t.Fatalf("link order %d produced %d resources, want the transitive explicit same_as component folded once: %+v", index, len(resources), resources)
		}
		if canonicalWithKomodo != pveIDs[0] {
			t.Fatalf("link order %d selected conflict survivor %q, want the earliest operator-selected Proxmox primary %q", index, canonicalWithKomodo, pveIDs[0])
		}
		targets := registry.SourceTargets(canonicalWithKomodo)
		found := map[unifiedresources.DataSource]map[string]bool{}
		for _, target := range targets {
			if found[target.Source] == nil {
				found[target.Source] = map[string]bool{}
			}
			found[target.Source][target.SourceID] = true
		}
		if !found[unifiedresources.SourceKomodo][komodoSourceID] || !found[unifiedresources.SourceProxmox][pveSourceIDs[0]] || !found[unifiedresources.SourceProxmox][pveSourceIDs[1]] {
			t.Fatalf("transitive explicit link source identities = %v, want Komodo %q and both Proxmox IDs %v", found, komodoSourceID, pveSourceIDs)
		}
		metricTarget := registry.MetricsTarget(canonicalWithKomodo)
		if metricTarget == nil || metricTarget.ResourceID != pveSourceIDs[0] {
			t.Fatalf("canonical history target = %+v, want canonical Proxmox source %q", metricTarget, pveSourceIDs[0])
		}
		if index == 0 {
			firstCanonical = canonicalWithKomodo
		} else if canonicalWithKomodo != firstCanonical {
			t.Fatalf("conflicting link order changed canonical choice from %q to %q", firstCanonical, canonicalWithKomodo)
		}
	}
}

func linkedEstateRecords(connectionID, pveSourceID, serverID, swarmID, probeID string, observedAt time.Time) ([]unifiedresources.IngestRecord, []unifiedresources.IngestRecord, []unifiedresources.IngestRecord) {
	pve := []unifiedresources.IngestRecord{{
		SourceID: pveSourceID,
		Resource: unifiedresources.Resource{
			Type: unifiedresources.ResourceTypeVM, Name: "Jellyfin-DMZ", Status: unifiedresources.StatusOnline,
			LastSeen: observedAt, Proxmox: &unifiedresources.ProxmoxData{VMID: 104, Instance: "cluster-a"},
		},
	}}
	komodo := NormalizeInventory(connectionID, Inventory{
		Servers: []Resource{{ID: serverID, Name: "Jellyfin-DMZ", Kind: KindServer, State: "Ok"}},
		Swarms:  []Resource{{ID: swarmID, Name: "production", Kind: KindSwarm, State: "Healthy", ServerIDs: []string{serverID}}},
		Stacks:  []Resource{{ID: "stack-jellyfin", Name: "jellyfin", Kind: KindStack, State: "Running", ServerID: serverID}},
	}, observedAt)
	availability := []unifiedresources.IngestRecord{{
		SourceID: probeID,
		Resource: unifiedresources.Resource{
			Type: unifiedresources.ResourceTypeNetworkEndpoint, Name: "Jellyfin check", Status: unifiedresources.StatusOnline,
			LastSeen: observedAt, Sources: []unifiedresources.DataSource{unifiedresources.SourceAvailability},
			Availability: &unifiedresources.AvailabilityData{
				TargetID: probeID, LinkedResourceID: unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeKomodoServer, unifiedresources.SourceKomodo, SourceID(connectionID, KindServer, serverID)),
				Address: "192.0.2.104", Protocol: "tcp", Enabled: true, Available: true,
			},
		},
	}}
	return pve, komodo, availability
}

func assertLinkedEstate(t *testing.T, registry *unifiedresources.ResourceRegistry, pveID, komodoID, komodoSourceID, pveSourceID, swarmID, probeID string) {
	t.Helper()
	vm, ok := registry.Get(pveID)
	if !ok || vm == nil {
		t.Fatalf("canonical Proxmox resource %q missing after identity fold", pveID)
	}
	if vm.ID != pveID || vm.Type != unifiedresources.ResourceTypeVM {
		t.Fatalf("canonical resource = %s/%s, want selected Proxmox VM identity %s", vm.ID, vm.Type, pveID)
	}
	if !hasSource(vm.Sources, unifiedresources.SourceProxmox) || !hasSource(vm.Sources, unifiedresources.SourceKomodo) {
		t.Fatalf("canonical sources = %v, want both providers", vm.Sources)
	}
	sourceIDs := make(map[unifiedresources.DataSource]string)
	for _, target := range registry.SourceTargets(pveID) {
		sourceIDs[target.Source] = target.SourceID
	}
	if sourceIDs[unifiedresources.SourceProxmox] != pveSourceID || sourceIDs[unifiedresources.SourceKomodo] != komodoSourceID {
		t.Fatalf("provider source identities = %v, want Proxmox %q and Komodo %q", sourceIDs, pveSourceID, komodoSourceID)
	}
	if target := registry.MetricsTarget(pveID); target == nil || target.ResourceType != "vm" || target.ResourceID != pveSourceID {
		t.Fatalf("history target = %+v, want vm/%s", target, pveSourceID)
	}

	var stack *unifiedresources.Resource
	for _, candidate := range registry.ListByType(unifiedresources.ResourceTypeKomodoStack) {
		copy := candidate
		stack = &copy
	}
	if stack == nil || stack.ParentID == nil || *stack.ParentID != pveID {
		t.Fatalf("Komodo Stack parent = %+v, want canonical host %q", stack, pveID)
	}
	stackEdges := unifiedresources.ResourceRelationshipsWithCanonicalParent(*stack)
	if len(stackEdges) != 1 || stackEdges[0].Type != unifiedresources.RelHostedBy || stackEdges[0].SourceID != stack.ID || stackEdges[0].TargetID != pveID {
		t.Fatalf("Komodo Stack edges = %+v, want hosted_by canonical Proxmox VM", stackEdges)
	}
	var hasMembership bool
	for _, edge := range vm.Relationships {
		if edge.Type == unifiedresources.RelMemberOf && edge.SourceID == pveID && edge.TargetID == swarmID {
			hasMembership = true
		}
	}
	if !hasMembership {
		t.Fatalf("Proxmox canonical resource relationships = %+v, want rewritten Komodo member_of Swarm edge to %q", vm.Relationships, swarmID)
	}

	checks := unifiedresources.AvailabilityChecksForResource(*vm)
	if len(checks) != 1 || checks[0].TargetID != probeID {
		endpoints := registry.ListByType(unifiedresources.ResourceTypeNetworkEndpoint)
		t.Fatalf("availability checks on linked VM = %+v, want probe %q; endpoints = %+v", checks, probeID, endpoints)
	}
	var endpoint *unifiedresources.Resource
	for _, candidate := range registry.ListByType(unifiedresources.ResourceTypeNetworkEndpoint) {
		if candidate.Availability != nil && candidate.Availability.TargetID == probeID {
			copy := candidate
			endpoint = &copy
		}
	}
	if endpoint == nil {
		t.Fatalf("availability endpoint %q missing", probeID)
	}
	for _, edge := range endpoint.Relationships {
		if edge.Type == unifiedresources.RelChecks && edge.TargetID == pveID {
			return
		}
	}
	t.Fatalf("availability endpoint relationships = %+v, want checks edge to canonical VM %q (Komodo ID was %q)", endpoint.Relationships, pveID, komodoID)
}

func hasSource(sources []unifiedresources.DataSource, target unifiedresources.DataSource) bool {
	for _, source := range sources {
		if source == target {
			return true
		}
	}
	return false
}
