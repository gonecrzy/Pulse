package komodo

import (
	"reflect"
	"testing"
	"time"

	"github.com/rcourtman/pulse-go-rewrite/internal/unifiedresources"
)

func TestNormalizeInventoryScopesStableIdentityByConnectionKindAndNativeID(t *testing.T) {
	inventory := Inventory{
		Servers: []Resource{{ID: "same", Name: "same-name", Kind: KindServer, State: "Ok"}},
		Stacks:  []Resource{{ID: "same", Name: "same-name", Kind: KindStack, State: "Running"}},
	}
	first := NormalizeInventory("org-connection-a", inventory, time.Unix(100, 0))
	second := NormalizeInventory("org-connection-b", inventory, time.Unix(100, 0))
	if len(first) != 2 || len(second) != 2 {
		t.Fatalf("normalized record counts = %d and %d, want 2", len(first), len(second))
	}
	if first[0].SourceID == first[1].SourceID || first[0].SourceID == second[0].SourceID {
		t.Fatal("Komodo source ID did not include kind and connection identity")
	}
	if !reflect.DeepEqual(first[0].Identity, unifiedresources.ResourceIdentity{}) || !reflect.DeepEqual(first[1].Identity, unifiedresources.ResourceIdentity{}) {
		t.Fatal("Komodo records must not infer cross-provider identity from names")
	}
	if first[0].Resource.Type == first[1].Resource.Type || first[0].Resource.Type != "komodo-server" || first[1].Resource.Type != "komodo-stack" {
		t.Fatalf("resource types = %q and %q, want separate Komodo kinds", first[0].Resource.Type, first[1].Resource.Type)
	}
}

func TestNormalizeInventorySetsParentsAndSwarmMembershipWithoutRawPayload(t *testing.T) {
	now := time.Unix(200, 0).UTC()
	inventory := Inventory{
		Servers:     []Resource{{ID: "server-1", Name: "worker", Kind: KindServer, State: "Ok"}},
		Swarms:      []Resource{{ID: "swarm-1", Name: "cluster", Kind: KindSwarm, State: "Healthy", ServerIDs: []string{"server-1"}}},
		Stacks:      []Resource{{ID: "stack-1", Name: "web", Kind: KindStack, State: "Running", ServerID: "server-1"}},
		Deployments: []Resource{{ID: "deployment-1", Name: "api", Kind: KindDeployment, State: "Deploying", SwarmID: "swarm-1", ServerID: "server-1"}},
	}
	records := NormalizeInventory("connection-1", inventory, now)
	byKind := make(map[unifiedresources.ResourceType]unifiedresources.IngestRecord)
	for _, record := range records {
		byKind[record.Resource.Type] = record
		if record.Resource.LastSeen != now || record.Resource.UpdatedAt != now {
			t.Fatalf("record timestamps = %v/%v, want %v", record.Resource.LastSeen, record.Resource.UpdatedAt, now)
		}
	}
	stack := byKind["komodo-stack"]
	if stack.ParentSourceID != SourceID("connection-1", KindServer, "server-1") || len(stack.Resource.Relationships) != 1 || stack.Resource.Relationships[0].Type != unifiedresources.RelHostedBy {
		t.Fatalf("stack parent mapping = %#v, want server parent and hosted_by edge", stack)
	}
	deployment := byKind["komodo-deployment"]
	if deployment.ParentSourceID != SourceID("connection-1", KindSwarm, "swarm-1") || len(deployment.Resource.Relationships) != 1 {
		t.Fatalf("deployment parent mapping = %#v, want swarm precedence", deployment)
	}
	server := byKind["komodo-server"].Resource
	if len(server.Relationships) != 1 || server.Relationships[0].Type != unifiedresources.RelMemberOf {
		t.Fatalf("server relationships = %#v, want Swarm member_of edge", server.Relationships)
	}
	if server.Status != unifiedresources.StatusOnline || byKind["komodo-deployment"].Resource.Status != unifiedresources.StatusWarning {
		t.Fatalf("mapped statuses = %q and %q", server.Status, byKind["komodo-deployment"].Resource.Status)
	}
	if server.Capabilities != nil || server.ActionReadiness != nil || server.AISafeSummary != "" {
		t.Fatalf("Komodo server gained action or AI context fields: %#v", server)
	}
}

func TestKomodoRecordsEnterExistingRegistryAndRemainSeparateFromSameNamedResources(t *testing.T) {
	registry := unifiedresources.NewRegistry(nil)
	registry.IngestRecords(unifiedresources.SourceProxmox, []unifiedresources.IngestRecord{{
		SourceID: "pve-node-1",
		Resource: unifiedresources.Resource{Type: unifiedresources.ResourceTypeAgent, Name: "shared-name", Status: unifiedresources.StatusOnline, LastSeen: time.Now().UTC()},
	}})
	adapter := unifiedresources.NewMonitorAdapter(registry)
	komodoRecords := NormalizeInventory("connection-1", Inventory{
		Servers: []Resource{{ID: "server-1", Name: "shared-name", Kind: KindServer, State: "Ok"}},
	}, time.Now().UTC())
	adapter.PopulateSupplementalRecords(unifiedresources.SourceKomodo, komodoRecords)

	resources := adapter.GetAll()
	if len(resources) != 2 {
		t.Fatalf("registry resources = %d, want Proxmox and Komodo rows", len(resources))
	}
	var sources map[string]bool = make(map[string]bool)
	for _, resource := range resources {
		sources[string(resource.Sources[0])] = true
	}
	if !sources[string(unifiedresources.SourceProxmox)] || !sources[string(unifiedresources.SourceKomodo)] {
		t.Fatalf("registry sources = %v, want both existing Proxmox and Komodo records", sources)
	}
	if len(adapter.GetAll()) != len(resources) {
		t.Fatal("registry read changed after supplemental Komodo ingest")
	}
}

func TestKomodoRelationshipsFollowAnOperatorConfirmedProxmoxIdentityLink(t *testing.T) {
	const connectionID = "87ef53c3-9a5a-4d7b-a159-997ccca19811"
	const serverNativeID = "server-jellyfin"
	const swarmNativeID = "swarm-production"
	pveSourceID := "pve:cluster-a:vmid:104"
	komodoServerSourceID := SourceID(connectionID, KindServer, serverNativeID)
	pveCanonicalID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeVM, unifiedresources.SourceProxmox, pveSourceID)
	komodoCanonicalID := unifiedresources.SourceSpecificID(unifiedresources.ResourceTypeKomodoServer, unifiedresources.SourceKomodo, komodoServerSourceID)
	registry := unifiedresources.NewRegistryWithManualLinks([]unifiedresources.ResourceLink{{
		ResourceA: pveCanonicalID,
		ResourceB: komodoCanonicalID,
		PrimaryID: pveCanonicalID,
		Reason:    "operator-confirmed same VM",
		CreatedAt: time.Unix(1, 0).UTC(),
	}})
	adapter := unifiedresources.NewMonitorAdapter(registry)
	observedAt := time.Unix(200, 0).UTC()
	adapter.PopulateSupplementalRecords(unifiedresources.SourceProxmox, []unifiedresources.IngestRecord{{
		SourceID: pveSourceID,
		Resource: unifiedresources.Resource{
			Type: unifiedresources.ResourceTypeVM, Name: "Jellyfin-DMZ", Status: unifiedresources.StatusOnline,
			LastSeen: observedAt, Proxmox: &unifiedresources.ProxmoxData{VMID: 104, Instance: "cluster-a"},
		},
	}})
	adapter.PopulateSupplementalRecords(unifiedresources.SourceKomodo, NormalizeInventory(connectionID, Inventory{
		Servers: []Resource{{ID: serverNativeID, Name: "Jellyfin-DMZ", Kind: KindServer, State: "Ok"}},
		Swarms:  []Resource{{ID: swarmNativeID, Name: "production", Kind: KindSwarm, State: "Healthy", ServerIDs: []string{serverNativeID}}},
		Stacks:  []Resource{{ID: "stack-jellyfin", Name: "jellyfin", Kind: KindStack, State: "Running", ServerID: serverNativeID}},
	}, observedAt))

	var vm, stack unifiedresources.Resource
	for _, resource := range adapter.GetAll() {
		if resource.ID == pveCanonicalID {
			vm = resource
		}
		if resource.Type == unifiedresources.ResourceTypeKomodoStack {
			stack = resource
		}
	}
	if vm.ID != pveCanonicalID || len(vm.Sources) != 2 {
		t.Fatalf("operator link did not merge Komodo Server into the Proxmox VM: %+v", vm)
	}
	if stack.ParentID == nil || *stack.ParentID != pveCanonicalID {
		t.Fatalf("Komodo Stack parent = %v, want canonical Proxmox VM %q", stack.ParentID, pveCanonicalID)
	}
	stackRelationships := unifiedresources.ResourceRelationshipsWithCanonicalParent(stack)
	if len(stackRelationships) != 1 || stackRelationships[0].Type != unifiedresources.RelHostedBy || stackRelationships[0].TargetID != pveCanonicalID {
		t.Fatalf("Stack relationships = %+v, want one hosted_by edge to the linked VM", stackRelationships)
	}
	var membership *unifiedresources.ResourceRelationship
	for _, relationship := range vm.Relationships {
		if relationship.Type == unifiedresources.RelMemberOf {
			copy := relationship
			membership = &copy
		}
	}
	if membership == nil || membership.SourceID != pveCanonicalID || membership.TargetID == "" {
		t.Fatalf("linked server lost or misdirected Swarm membership: %+v", vm.Relationships)
	}
}
