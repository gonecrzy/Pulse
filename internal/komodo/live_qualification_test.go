package komodo

import (
	"context"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/rcourtman/pulse-go-rewrite/internal/config"
)

// TestLiveCoreV2Qualification is deliberately opt-in. It reads an existing
// encrypted Komodo connection from an explicitly selected Pulse data
// directory and performs only the inventory client's allowlisted read calls.
// It never prints the endpoint, connection ID, API key, API secret, names, or
// native resource IDs.
func TestLiveCoreV2Qualification(t *testing.T) {
	if os.Getenv("PULSE_KOMODO_LIVE_QUALIFICATION") != "1" {
		t.Skip("set PULSE_KOMODO_LIVE_QUALIFICATION=1 to opt in to live read-only qualification")
	}
	dataDir := strings.TrimSpace(os.Getenv("PULSE_DATA_DIR"))
	connectionID := strings.TrimSpace(os.Getenv("PULSE_KOMODO_QUALIFICATION_CONNECTION_ID"))
	if dataDir == "" || connectionID == "" {
		t.Fatal("live qualification requires an explicit PULSE_DATA_DIR and connection ID")
	}

	persistence := config.NewConfigPersistence(dataDir)
	instances, err := persistence.LoadKomodoConfig()
	if err != nil {
		t.Fatal("could not load protected Komodo connection configuration")
	}
	var selected *config.KomodoInstance
	for index := range instances {
		if instances[index].ID == connectionID {
			selected = &instances[index]
			break
		}
	}
	if selected == nil {
		t.Fatal("selected Komodo connection was not found in the protected configuration")
	}
	if !selected.Enabled {
		t.Fatal("selected Komodo connection is disabled")
	}
	if err := selected.Validate(); err != nil {
		t.Fatal("selected Komodo connection configuration is invalid")
	}

	client, err := NewClient(ClientConfig{
		Endpoint:  selected.Endpoint,
		APIKey:    selected.APIKey,
		APISecret: selected.APISecret,
	})
	if err != nil {
		t.Fatal("could not create the bounded Komodo read client")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	first, err := client.CollectInventory(ctx)
	if err != nil {
		t.Fatalf("first inventory collection failed safely: %v", err)
	}
	second, err := client.CollectInventory(ctx)
	if err != nil {
		t.Fatalf("second inventory collection failed safely: %v", err)
	}

	firstIDs := inventoryIdentitySet(connectionID, first)
	secondIDs := inventoryIdentitySet(connectionID, second)
	identitySetsStable := equalStringSets(firstIDs, secondIDs)
	parentEvidence := summarizeParentEvidence(first)

	t.Logf("Komodo live qualification: Core=%s scope_verified=%d/%d counts(server=%d stack=%d deployment=%d swarm=%d) identity_sets_stable=%t parent_fields(stack_server=%d stack_swarm=%d deployment_server=%d deployment_swarm=%d swarm_members=%d unresolved_parent_refs=%d)",
		first.Version,
		len(first.Scope.Verified), len(resourceKinds),
		len(first.Servers), len(first.Stacks), len(first.Deployments), len(first.Swarms),
		identitySetsStable,
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
