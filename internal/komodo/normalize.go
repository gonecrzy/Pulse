package komodo

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/rcourtman/pulse-go-rewrite/internal/storagehealth"
	"github.com/rcourtman/pulse-go-rewrite/internal/unifiedresources"
)

// ResourceType maps a Komodo kind to a disjoint unified-resource type. These
// types intentionally do not alias Docker, Proxmox, or Pulse agent resources.
func ResourceType(kind ResourceKind) unifiedresources.ResourceType {
	switch kind {
	case KindServer:
		return unifiedresources.ResourceTypeKomodoServer
	case KindStack:
		return unifiedresources.ResourceTypeKomodoStack
	case KindDeployment:
		return unifiedresources.ResourceTypeKomodoDeployment
	case KindSwarm:
		return unifiedresources.ResourceTypeKomodoSwarm
	default:
		return ""
	}
}

// SourceID builds an opaque, stable source key scoped by connection UUID,
// Komodo resource kind, and Komodo's native resource ID.
func SourceID(connectionID string, kind ResourceKind, nativeID string) string {
	connectionID = strings.TrimSpace(connectionID)
	nativeID = strings.TrimSpace(nativeID)
	if connectionID == "" || nativeID == "" || ResourceType(kind) == "" {
		return ""
	}
	encode := base64.RawURLEncoding.EncodeToString
	return fmt.Sprintf("komodo:%s:%s:%s", encode([]byte(connectionID)), strings.ToLower(string(kind)), encode([]byte(nativeID)))
}

// NormalizeInventory converts the safe client inventory into source-native
// supplemental records. It intentionally does not derive identity from
// resource names; each record remains isolated to its connection and kind.
func NormalizeInventory(connectionID string, inventory Inventory, observedAt time.Time) []unifiedresources.IngestRecord {
	connectionID = strings.TrimSpace(connectionID)
	if connectionID == "" {
		return nil
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	} else {
		observedAt = observedAt.UTC()
	}

	serverIDs := make(map[string]string, len(inventory.Servers))
	swarmIDs := make(map[string]string, len(inventory.Swarms))
	for _, resource := range inventory.Servers {
		serverIDs[resource.ID] = SourceID(connectionID, KindServer, resource.ID)
	}
	for _, resource := range inventory.Swarms {
		swarmIDs[resource.ID] = SourceID(connectionID, KindSwarm, resource.ID)
	}

	records := make([]unifiedresources.IngestRecord, 0, inventory.Count())
	appendResource := func(item Resource, parentKind ResourceKind, parentID string) {
		resourceType := ResourceType(item.Kind)
		sourceID := SourceID(connectionID, item.Kind, item.ID)
		if resourceType == "" || sourceID == "" {
			return
		}
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = item.ID
		}
		resource := unifiedresources.Resource{
			Type:       resourceType,
			Technology: "komodo",
			Name:       name,
			Status:     statusFromState(item.State),
			LastSeen:   observedAt,
			UpdatedAt:  observedAt,
		}
		if incident, ok := healthIncident(item, observedAt); ok {
			resource.Incidents = []unifiedresources.ResourceIncident{incident}
			resource.IncidentCount = 1
			resource.IncidentCode = incident.Code
			resource.IncidentSeverity = incident.Severity
			resource.IncidentSummary = incident.Summary
			resource.IncidentCategory = "komodo-resource-health"
		}
		parentSourceID := ""
		if parentID != "" && parentKind != "" {
			parentSourceID = SourceID(connectionID, parentKind, parentID)
			parentPresent := false
			switch parentKind {
			case KindServer:
				parentPresent = strings.TrimSpace(serverIDs[parentID]) != ""
			case KindSwarm:
				parentPresent = strings.TrimSpace(swarmIDs[parentID]) != ""
			}
			if parentPresent && parentSourceID != "" {
				parentType := ResourceType(parentKind)
				parentCanonicalID := unifiedresources.SourceSpecificID(parentType, unifiedresources.SourceKomodo, parentSourceID)
				resource.Relationships = append(resource.Relationships, unifiedresources.ResourceRelationship{
					SourceID:       unifiedresources.SourceSpecificID(resourceType, unifiedresources.SourceKomodo, sourceID),
					TargetID:       parentCanonicalID,
					Type:           unifiedresources.RelHostedBy,
					Confidence:     1,
					EvidenceSource: unifiedresources.SourceKomodo,
					Active:         true,
					Discoverer:     "komodo_adapter",
					ObservedAt:     observedAt,
					LastSeenAt:     observedAt,
				})
			}
		}
		records = append(records, unifiedresources.IngestRecord{
			SourceID:       sourceID,
			ParentSourceID: parentSourceID,
			Resource:       resource,
		})
	}

	// Parent records precede children so ParentSourceID resolves during the
	// same registry ingestion pass.
	for _, item := range inventory.Servers {
		appendResource(item, "", "")
	}
	for _, item := range inventory.Swarms {
		appendResource(item, "", "")
	}
	for _, item := range inventory.Stacks {
		parentKind, parentID := stackParent(item)
		appendResource(item, parentKind, parentID)
	}
	for _, item := range inventory.Deployments {
		parentKind, parentID := stackParent(item)
		appendResource(item, parentKind, parentID)
	}

	// Komodo models Swarm membership as a list on the Swarm. Project it as
	// member_of edges from each known server to the Swarm, preserving the
	// multi-parent relation without manufacturing a singular parent.
	recordBySource := make(map[string]int, len(records))
	for index := range records {
		recordBySource[records[index].SourceID] = index
	}
	for _, swarm := range inventory.Swarms {
		swarmSource := swarmIDs[swarm.ID]
		swarmCanonicalID := unifiedresources.SourceSpecificID(ResourceType(KindSwarm), unifiedresources.SourceKomodo, swarmSource)
		for _, serverID := range swarm.ServerIDs {
			serverSource := serverIDs[serverID]
			index, exists := recordBySource[serverSource]
			if !exists || serverSource == "" {
				continue
			}
			serverCanonicalID := unifiedresources.SourceSpecificID(ResourceType(KindServer), unifiedresources.SourceKomodo, serverSource)
			records[index].Resource.Relationships = append(records[index].Resource.Relationships, unifiedresources.ResourceRelationship{
				SourceID:   serverCanonicalID,
				TargetID:   swarmCanonicalID,
				Type:       unifiedresources.RelMemberOf,
				Confidence: 1,
				Active:     true,
				Discoverer: "komodo_adapter",
				ObservedAt: observedAt,
				LastSeenAt: observedAt,
			})
		}
	}
	return records
}

// healthIncident turns only explicit adverse Komodo states into a canonical
// incident. Stopped/disabled and transitional states retain their normalized
// resource status without becoming notification-worthy failures.
func healthIncident(resource Resource, observedAt time.Time) (unifiedresources.ResourceIncident, bool) {
	state := strings.ToLower(strings.TrimSpace(resource.State))
	severity := storagehealth.RiskLevel("")
	switch state {
	case "notok", "unhealthy", "offline", "down", "failed", "failure", "error", "errored", "notdeployed":
		severity = storagehealth.RiskCritical
	case "warning":
		severity = storagehealth.RiskWarning
	default:
		return unifiedresources.ResourceIncident{}, false
	}
	kind := strings.ToLower(strings.TrimSpace(string(resource.Kind)))
	if kind == "" {
		kind = "resource"
	}
	return unifiedresources.ResourceIncident{
		Provider:  "komodo",
		NativeID:  strings.TrimSpace(resource.ID),
		Code:      "komodo_resource_unhealthy",
		Severity:  severity,
		Source:    "komodo-poller",
		Summary:   "Komodo reported an unhealthy " + kind,
		StartedAt: observedAt,
	}, true
}

func stackParent(resource Resource) (ResourceKind, string) {
	if strings.TrimSpace(resource.SwarmID) != "" {
		return KindSwarm, strings.TrimSpace(resource.SwarmID)
	}
	if strings.TrimSpace(resource.ServerID) != "" {
		return KindServer, strings.TrimSpace(resource.ServerID)
	}
	return "", ""
}

func statusFromState(state string) unifiedresources.ResourceStatus {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "ok", "healthy", "running", "deployed", "ready":
		return unifiedresources.StatusOnline
	case "notok", "unhealthy", "offline", "stopped", "failed", "notdeployed", "disabled", "down":
		return unifiedresources.StatusOffline
	case "deploying", "starting", "stopping", "restarting", "paused", "warning", "initializing":
		return unifiedresources.StatusWarning
	default:
		return unifiedresources.StatusUnknown
	}
}
