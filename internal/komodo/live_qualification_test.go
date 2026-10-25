package komodo

import (
	"context"
	"errors"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/rcourtman/pulse-go-rewrite/internal/unifiedresources"
)

// TestLiveCoreV2Qualification is deliberately opt-in. It reads an existing
// encrypted Komodo connection from an explicitly selected Pulse data
// directory when available, otherwise it may read an owner-only test
// credential file. It performs only the inventory client's allowlisted read
// calls and never prints the endpoint, connection ID, key, secret, names, or
// native resource IDs.
func TestLiveCoreV2Qualification(t *testing.T) {
	if os.Getenv("PULSE_KOMODO_LIVE_QUALIFICATION") != "1" {
		t.Skip("set PULSE_KOMODO_LIVE_QUALIFICATION=1 to opt in to live read-only qualification")
	}
	selected, err := loadLiveQualificationConfig()
	if err != nil {
		t.Fatal(err)
	}

	client, err := newLiveQualificationClient(selected.Client)
	if err != nil {
		t.Fatal("could not create the bounded Komodo read client")
	}
	var permissionTransport *qualificationPermissionTransport
	attestationPath := strings.TrimSpace(os.Getenv("PULSE_KOMODO_QUALIFICATION_ATTESTATION_FILE"))
	var permissionAttestation qualificationPermissionAttestation
	if attestationPath == "" {
		t.Logf("No permission attestation file was selected; non-secret credential fingerprint for operator verification: %s", qualificationCredentialFingerprint(selected.Client))
	}
	if attestationPath != "" {
		permissionAttestation, err = loadQualificationPermissionAttestation(attestationPath, selected.Client, time.Now().UTC())
		if err != nil {
			t.Fatalf("permission attestation could not be accepted safely: %v", err)
		}
	}
	permissionTransport = newQualificationPermissionTransport(client.httpClient.Transport, permissionAttestation)
	client.httpClient.Transport = permissionTransport

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var version versionResponse
	if err := client.call(ctx, "GetVersion", struct{}{}, &version); err != nil {
		t.Fatalf("Core version check failed safely: %v", err)
	}
	t.Logf("Komodo Core version reported: %s", version.Version)
	first, err := client.CollectInventory(ctx)
	if err != nil {
		t.Fatalf("first inventory collection failed safely: %v", err)
	}
	if len(first.Scope.SpecificPermissionsUnverified) > 0 && !permissionTransport.attested {
		t.Fatalf("inventory read methods succeeded, but effective specific permissions remain unverified for %d resource kinds; the live qualification gate requires the existing operator attestation", len(first.Scope.SpecificPermissionsUnverified))
	}
	second, err := client.CollectInventory(ctx)
	if err != nil {
		t.Fatalf("second inventory collection failed safely: %v", err)
	}

	firstIDs := inventoryIdentitySet(selected.IdentityID, first)
	secondIDs := inventoryIdentitySet(selected.IdentityID, second)
	identitySetsStable := equalStringSets(firstIDs, secondIDs)
	parentEvidence := summarizeParentEvidence(first)
	stateEvidence := summarizeStateEvidence(first)
	parentEvidenceStable := parentEvidence == summarizeParentEvidence(second)
	normalizedEvidence, err := validateNormalizedInventory(selected.IdentityID, first)
	if err != nil {
		t.Fatalf("live inventory did not satisfy the Pulse normalization contract: %v", err)
	}

	permissionEvidence := "api_verified"
	if len(first.Scope.SpecificPermissionsUnverified) > 0 {
		permissionEvidence = "operator_attested_for_specifics"
	}
	t.Logf("Komodo live qualification: Core=%s permission_evidence=%s level_only_read_responses=%d api_read_scope=%d/%d api_specific_scope=%d/%d operator_attested_specific_scope=%d counts(server=%d stack=%d deployment=%d swarm=%d) identities=%d identity_sets_stable=%t resources_with_state(server=%d stack=%d deployment=%d swarm=%d) parent_fields(stack_server=%d stack_swarm=%d deployment_server=%d deployment_swarm=%d swarm_members=%d unresolved_parent_refs=%d) parent_evidence_stable=%t normalized_records=%d normalized_status(online=%d warning=%d offline=%d unknown=%d) normalized_incidents=%d normalized_hosted_by=%d normalized_member_of=%d",
		first.Version,
		permissionEvidence, permissionTransport.seen,
		len(first.Scope.Verified), len(resourceKinds),
		len(first.Scope.SpecificPermissionsVerified), len(resourceKinds),
		len(first.Scope.SpecificPermissionsUnverified),
		len(first.Servers), len(first.Stacks), len(first.Deployments), len(first.Swarms),
		len(firstIDs), identitySetsStable,
		stateEvidence.servers, stateEvidence.stacks, stateEvidence.deployments, stateEvidence.swarms,
		parentEvidence.stackServer, parentEvidence.stackSwarm,
		parentEvidence.deploymentServer, parentEvidence.deploymentSwarm,
		parentEvidence.swarmMembers, parentEvidence.unresolved,
		parentEvidenceStable, normalizedEvidence.records,
		normalizedEvidence.online, normalizedEvidence.warning, normalizedEvidence.offline, normalizedEvidence.unknown,
		normalizedEvidence.incidents, normalizedEvidence.hostedBy, normalizedEvidence.memberOf,
	)
	if first.Version != second.Version {
		t.Error("Komodo Core version changed between consecutive inventory reads")
	}
	if !identitySetsStable {
		t.Error("native inventory identity sets changed between consecutive reads; rerun during a stable inventory window")
	}
	if !parentEvidenceStable {
		t.Error("parent field evidence changed between consecutive inventory reads; rerun during a stable inventory window")
	}
}

// TestLiveQualificationCredentialFingerprintOnly verifies the current
// protected credential's non-secret binding without making a network request.
// It is useful while live permission qualification is paused for operator
// review and requires both explicit opt-in environment flags.
func TestLiveQualificationCredentialFingerprintOnly(t *testing.T) {
	if os.Getenv("PULSE_KOMODO_QUALIFICATION_FINGERPRINT_ONLY") != "1" {
		t.Skip("set PULSE_KOMODO_QUALIFICATION_FINGERPRINT_ONLY=1 to inspect the non-secret credential fingerprint")
	}
	if os.Getenv("PULSE_KOMODO_LIVE_QUALIFICATION") != "1" {
		t.Fatal("fingerprint inspection requires explicit live qualification opt-in")
	}
	selected, err := loadLiveQualificationConfig()
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := qualificationCredentialFingerprint(selected.Client)
	if fingerprint == "" {
		t.Fatal("could not derive a non-secret qualification credential fingerprint")
	}
	t.Logf("Non-secret Komodo qualification credential fingerprint: %s", fingerprint)
}

func TestSummarizeParentEvidenceHonorsSwarmParentPrecedence(t *testing.T) {
	inventory := Inventory{
		Swarms: []Resource{{ID: "swarm-present"}},
		Stacks: []Resource{
			{ServerID: "server-not-listed", SwarmID: "swarm-present"},
			{ServerID: "server-not-listed"},
		},
	}

	summary := summarizeParentEvidence(inventory)
	if summary.stackServer != 2 || summary.stackSwarm != 1 {
		t.Fatalf("parent field counts = server:%d swarm:%d, want server:2 swarm:1", summary.stackServer, summary.stackSwarm)
	}
	if summary.unresolved != 1 {
		t.Fatalf("unresolved parent references = %d, want 1 for the selected server-only parent", summary.unresolved)
	}
}

func inventoryIdentitySet(connectionID string, inventory Inventory) []string {
	identities := make([]string, 0, inventory.Count())
	appendItems := func(kind ResourceKind, items []Resource) {
		for _, item := range items {
			identities = append(identities, SourceID(connectionID, kind, item.ID))
		}
	}
	appendItems(KindServer, inventory.Servers)
	appendItems(KindStack, inventory.Stacks)
	appendItems(KindDeployment, inventory.Deployments)
	appendItems(KindSwarm, inventory.Swarms)
	sort.Strings(identities)
	return identities
}

func equalStringSets(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

type stateEvidenceSummary struct {
	servers, stacks, deployments, swarms int
}

func summarizeStateEvidence(inventory Inventory) stateEvidenceSummary {
	countReported := func(items []Resource) int {
		count := 0
		for _, item := range items {
			if item.State != "" {
				count++
			}
		}
		return count
	}
	return stateEvidenceSummary{
		servers:     countReported(inventory.Servers),
		stacks:      countReported(inventory.Stacks),
		deployments: countReported(inventory.Deployments),
		swarms:      countReported(inventory.Swarms),
	}
}

func TestSummarizeStateEvidenceReportsOnlyPresenceCounts(t *testing.T) {
	got := summarizeStateEvidence(Inventory{
		Servers:     []Resource{{State: "running"}, {State: ""}},
		Stacks:      []Resource{{State: "configured-secret-must-not-be-logged"}},
		Deployments: []Resource{{State: "active"}, {State: "stopped"}},
		Swarms:      []Resource{{State: ""}},
	})
	if got.servers != 1 || got.stacks != 1 || got.deployments != 2 || got.swarms != 0 {
		t.Fatal("state evidence did not count non-empty states by kind")
	}
}

type parentEvidenceSummary struct {
	stackServer, stackSwarm           int
	deploymentServer, deploymentSwarm int
	swarmMembers, unresolved          int
}

func summarizeParentEvidence(inventory Inventory) parentEvidenceSummary {
	var summary parentEvidenceSummary
	servers := make(map[string]struct{}, len(inventory.Servers))
	swarms := make(map[string]struct{}, len(inventory.Swarms))
	for _, item := range inventory.Servers {
		servers[item.ID] = struct{}{}
	}
	for _, item := range inventory.Swarms {
		swarms[item.ID] = struct{}{}
	}
	checkParent := func(serverID, swarmID string) {
		if swarmID != "" {
			if _, ok := swarms[swarmID]; !ok {
				summary.unresolved++
			}
			return
		}
		if serverID != "" {
			if _, ok := servers[serverID]; !ok {
				summary.unresolved++
			}
		}
	}
	for _, item := range inventory.Stacks {
		if item.ServerID != "" {
			summary.stackServer++
		}
		if item.SwarmID != "" {
			summary.stackSwarm++
		}
		checkParent(item.ServerID, item.SwarmID)
	}
	for _, item := range inventory.Deployments {
		if item.ServerID != "" {
			summary.deploymentServer++
		}
		if item.SwarmID != "" {
			summary.deploymentSwarm++
		}
		checkParent(item.ServerID, item.SwarmID)
	}
	for _, item := range inventory.Swarms {
		for _, serverID := range item.ServerIDs {
			summary.swarmMembers++
			if _, ok := servers[serverID]; !ok {
				summary.unresolved++
			}
		}
	}
	return summary
}

type normalizedInventorySummary struct {
	records, online, warning, offline, unknown int
	incidents, hostedBy, memberOf              int
}

func validateNormalizedInventory(connectionID string, inventory Inventory) (normalizedInventorySummary, error) {
	records := NormalizeInventory(connectionID, inventory, time.Now().UTC())
	if len(records) != inventory.Count() {
		return normalizedInventorySummary{}, errors.New("record count does not match the API inventory")
	}

	items := make(map[string]Resource, inventory.Count())
	for _, item := range inventory.Servers {
		items[SourceID(connectionID, KindServer, item.ID)] = item
	}
	for _, item := range inventory.Swarms {
		items[SourceID(connectionID, KindSwarm, item.ID)] = item
	}
	for _, item := range inventory.Stacks {
		items[SourceID(connectionID, KindStack, item.ID)] = item
	}
	for _, item := range inventory.Deployments {
		items[SourceID(connectionID, KindDeployment, item.ID)] = item
	}

	bySource := make(map[string]unifiedresources.IngestRecord, len(records))
	for _, record := range records {
		item, ok := items[record.SourceID]
		if !ok || record.SourceID == "" || record.Resource.Type != ResourceType(item.Kind) {
			return normalizedInventorySummary{}, errors.New("normalized record identity or resource type does not match the API item")
		}
		if _, duplicate := bySource[record.SourceID]; duplicate {
			return normalizedInventorySummary{}, errors.New("normalized inventory contains a duplicate source identity")
		}
		bySource[record.SourceID] = record
	}

	summary := normalizedInventorySummary{records: len(records)}
	for _, record := range records {
		item := items[record.SourceID]
		switch record.Resource.Status {
		case unifiedresources.StatusOnline:
			summary.online++
		case unifiedresources.StatusWarning:
			summary.warning++
		case unifiedresources.StatusOffline:
			summary.offline++
		default:
			summary.unknown++
		}
		summary.incidents += len(record.Resource.Incidents)
		parentKind, parentID := stackParent(item)
		if parentID == "" {
			continue
		}
		parentSource := SourceID(connectionID, parentKind, parentID)
		_, parentExists := bySource[parentSource]
		parentType := ResourceType(parentKind)
		parentCanonicalID := unifiedresources.SourceSpecificID(parentType, unifiedresources.SourceKomodo, parentSource)
		if !parentExists || record.ParentSourceID != parentSource || !hasQualifiedRelationship(record.Resource.Relationships, unifiedresources.RelHostedBy, parentCanonicalID) {
			return normalizedInventorySummary{}, errors.New("normalized parent relationship does not match the API parent fields")
		}
		summary.hostedBy++
	}

	for _, swarm := range inventory.Swarms {
		swarmSource := SourceID(connectionID, KindSwarm, swarm.ID)
		swarmCanonicalID := unifiedresources.SourceSpecificID(ResourceType(KindSwarm), unifiedresources.SourceKomodo, swarmSource)
		for _, nativeServerID := range swarm.ServerIDs {
			serverSource := SourceID(connectionID, KindServer, nativeServerID)
			serverRecord, serverExists := bySource[serverSource]
			if !serverExists {
				continue
			}
			if !hasQualifiedRelationship(serverRecord.Resource.Relationships, unifiedresources.RelMemberOf, swarmCanonicalID) {
				return normalizedInventorySummary{}, errors.New("normalized Swarm membership does not match the API member fields")
			}
			summary.memberOf++
		}
	}
	return summary, nil
}

func hasQualifiedRelationship(relationships []unifiedresources.ResourceRelationship, kind unifiedresources.RelationshipType, targetID string) bool {
	for _, relationship := range relationships {
		if relationship.Type == kind && relationship.TargetID == targetID && relationship.Active {
			return true
		}
	}
	return false
}

func TestValidateNormalizedInventoryChecksParentAndMembershipMappings(t *testing.T) {
	inventory := Inventory{
		Servers: []Resource{{ID: "server-1", Kind: KindServer, Name: "host", State: "Running"}},
		Swarms:  []Resource{{ID: "swarm-1", Kind: KindSwarm, Name: "cluster", ServerIDs: []string{"server-1"}}},
		Stacks: []Resource{
			{ID: "stack-1", Kind: KindStack, Name: "stack", SwarmID: "swarm-1"},
			{ID: "stack-2", Kind: KindStack, Name: "stack-standalone", ServerID: "server-1"},
		},
	}
	summary, err := validateNormalizedInventory("connection-1", inventory)
	if err != nil {
		t.Fatalf("validate normalized fixture: %v", err)
	}
	if summary.records != inventory.Count() || summary.hostedBy != 2 || summary.memberOf != 1 {
		t.Fatalf("normalized summary=%+v, want %d records, two hosted_by edges, and one member_of edge", summary, inventory.Count())
	}
}
