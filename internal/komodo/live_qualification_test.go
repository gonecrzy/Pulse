package komodo

import (
	"context"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
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

	client, err := NewClient(selected.Client)
	if err != nil {
		t.Fatal("could not create the bounded Komodo read client")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var version versionResponse
	if err := client.call(ctx, "GetVersion", struct{}{}, &version); err != nil {
		t.Fatalf("Core version check failed safely: %v", err)
	}
	t.Logf("Komodo Core version reported: %s", version.Version)
	first, err := client.CollectInventory(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "operator verification is required") {
			t.Log("Inventory counts, parent relationships, and identity stability were not qualified because permission scope verification stopped collection before returning inventory.")
			t.Fatalf("effective permission verification is incomplete: %v", err)
		}
		t.Fatalf("first inventory collection failed safely: %v", err)
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

	t.Logf("Komodo live qualification: Core=%s scope_verified=%d/%d counts(server=%d stack=%d deployment=%d swarm=%d) identities=%d identity_sets_stable=%t resources_with_state(server=%d stack=%d deployment=%d swarm=%d) parent_fields(stack_server=%d stack_swarm=%d deployment_server=%d deployment_swarm=%d swarm_members=%d unresolved_parent_refs=%d)",
		first.Version,
		len(first.Scope.Verified), len(resourceKinds),
		len(first.Servers), len(first.Stacks), len(first.Deployments), len(first.Swarms),
		len(firstIDs), identitySetsStable,
		stateEvidence.servers, stateEvidence.stacks, stateEvidence.deployments, stateEvidence.swarms,
		parentEvidence.stackServer, parentEvidence.stackSwarm,
		parentEvidence.deploymentServer, parentEvidence.deploymentSwarm,
		parentEvidence.swarmMembers, parentEvidence.unresolved,
	)
	if first.Version != second.Version {
		t.Error("Komodo Core version changed between consecutive inventory reads")
	}
	if !identitySetsStable {
		t.Error("native inventory identity sets changed between consecutive reads; rerun during a stable inventory window")
	}
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
