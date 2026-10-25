package unifiedresources

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"
)

const parentRelationshipDiscoverer = "resource_registry"

func mergeResourceRelationships(primary, incoming []ResourceRelationship) []ResourceRelationship {
	merged := make(map[string]ResourceRelationship, len(primary)+len(incoming))
	for _, batch := range [][]ResourceRelationship{primary, incoming} {
		for _, relationship := range batch {
			copy := relationship
			copy.Metadata = cloneRelationshipMetadata(relationship.Metadata)
			normalizeResourceRelationship(&copy)
			if copy.SourceID == "" || copy.TargetID == "" || copy.Type == "" || copy.SourceID == copy.TargetID {
				continue
			}
			current, exists := merged[copy.ID]
			if !exists || !copy.LastSeenAt.Before(current.LastSeenAt) {
				if exists && (current.ObservedAt.IsZero() || (!copy.ObservedAt.IsZero() && current.ObservedAt.Before(copy.ObservedAt))) {
					copy.ObservedAt = current.ObservedAt
				}
				merged[copy.ID] = copy
			}
		}
	}
	out := make([]ResourceRelationship, 0, len(merged))
	for _, relationship := range merged {
		out = append(out, relationship)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SourceID != out[j].SourceID {
			return out[i].SourceID < out[j].SourceID
		}
		if out[i].TargetID != out[j].TargetID {
			return out[i].TargetID < out[j].TargetID
		}
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		return out[i].Discoverer < out[j].Discoverer
	})
	return out
}

func rewriteResourceRelationshipEndpoint(relationships []ResourceRelationship, fromID, toID string) []ResourceRelationship {
	if len(relationships) == 0 || fromID == "" || toID == "" || fromID == toID {
		return relationships
	}
	rewritten := cloneResourceRelationshipSlice(relationships)
	for index := range rewritten {
		if rewritten[index].SourceID == fromID {
			rewritten[index].SourceID = toID
			rewritten[index].ID = ""
		}
		if rewritten[index].TargetID == fromID {
			rewritten[index].TargetID = toID
			rewritten[index].ID = ""
		}
	}
	return mergeResourceRelationships(nil, rewritten)
}

func cloneRelationshipMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	clone := make(map[string]any, len(metadata))
	for key, value := range metadata {
		clone[key] = value
	}
	return clone
}

// RelationshipType defines the connection semantics between two resources.
type RelationshipType string

const (
	RelRunsOn      RelationshipType = "runs_on"     // e.g., container runs_on machine
	RelDependsOn   RelationshipType = "depends_on"  // e.g., app depends_on database
	RelMountedTo   RelationshipType = "mounted_to"  // e.g., volume mounted_to container
	RelExposedBy   RelationshipType = "exposed_by"  // e.g., container exposed_by ingress
	RelOwnedBy     RelationshipType = "owned_by"    // e.g., pod owned_by deployment
	RelAttachedTo  RelationshipType = "attached_to" // e.g., container attached_to network
	RelChecks      RelationshipType = "checks"      // e.g., availability check checks resource
	RelHostedBy    RelationshipType = "hosted_by"
	RelStoresOn    RelationshipType = "stores_on"
	RelProtectedBy RelationshipType = "protected_by"
	RelMemberOf    RelationshipType = "member_of"
)

// ResourceRelationship represents a typed relationship edge between two unified resources.
type ResourceRelationship struct {
	ID         string           `json:"id,omitempty"`
	SourceID   string           `json:"sourceId"`
	TargetID   string           `json:"targetId"`
	Type       RelationshipType `json:"type"`
	Confidence float64          `json:"confidence"` // 1.0 for defined relationships, lower for heuristics
	EvidenceID string           `json:"evidenceId,omitempty"`

	// Provenance and time boundaries
	Active     bool           `json:"active"`     // False means historical edge
	Discoverer string         `json:"discoverer"` // e.g. "docker_adapter" or "pulse_inference_engine"
	ObservedAt time.Time      `json:"observedAt"`
	LastSeenAt time.Time      `json:"lastSeenAt"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// ResourceRelationshipsWithCanonicalParent returns explicit resource
// relationships plus a canonical relationship derived from ParentID when the
// resource has a parent but no equivalent typed edge.
func ResourceRelationshipsWithCanonicalParent(resource Resource) []ResourceRelationship {
	normalizeResourceRelationships(&resource)
	relationships := resource.Relationships

	sourceID := CanonicalResourceID(resource.ID)
	parentID := ""
	if resource.ParentID != nil {
		parentID = CanonicalResourceID(*resource.ParentID)
	}
	if sourceID == "" || parentID == "" || sourceID == parentID {
		return relationships
	}

	relationshipType := parentRelationshipType(resource.Type)
	if relationshipType == "" {
		return relationships
	}

	for _, relationship := range relationships {
		if CanonicalResourceID(relationship.SourceID) == sourceID &&
			CanonicalResourceID(relationship.TargetID) == parentID &&
			relationship.Type == relationshipType {
			return relationships
		}
	}

	observedAt := resource.UpdatedAt
	if observedAt.IsZero() {
		observedAt = resource.LastSeen
	}

	parent := ResourceRelationship{
		SourceID:   sourceID,
		TargetID:   parentID,
		Type:       relationshipType,
		Confidence: 1,
		Active:     true,
		Discoverer: parentRelationshipDiscoverer,
		ObservedAt: observedAt,
		LastSeenAt: observedAt,
		Metadata: map[string]any{
			"source": "parentId",
		},
	}
	normalizeResourceRelationship(&parent)
	return append(relationships, parent)
}

func normalizeResourceRelationships(resource *Resource) {
	if resource == nil || len(resource.Relationships) == 0 {
		return
	}
	for index := range resource.Relationships {
		normalizeResourceRelationship(&resource.Relationships[index])
	}
}

func normalizeResourceRelationship(relationship *ResourceRelationship) {
	if relationship == nil {
		return
	}
	relationship.SourceID = CanonicalResourceID(relationship.SourceID)
	relationship.TargetID = CanonicalResourceID(relationship.TargetID)
	relationship.Discoverer = strings.TrimSpace(relationship.Discoverer)
	relationship.EvidenceID = strings.TrimSpace(relationship.EvidenceID)
	if strings.TrimSpace(relationship.ID) != "" ||
		relationship.SourceID == "" ||
		relationship.TargetID == "" ||
		relationship.Type == "" {
		return
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{
		relationship.SourceID,
		relationship.TargetID,
		string(relationship.Type),
		relationship.Discoverer,
		relationship.EvidenceID,
	}, "\x00")))
	relationship.ID = "relationship_" + hex.EncodeToString(sum[:16])
}

func parentRelationshipType(resourceType ResourceType) RelationshipType {
	switch CanonicalResourceType(resourceType) {
	case ResourceTypeKomodoStack, ResourceTypeKomodoDeployment:
		return RelHostedBy
	case ResourceTypeVM,
		ResourceTypeSystemContainer,
		ResourceTypeAppContainer,
		ResourceTypeDockerService,
		ResourceTypePBS,
		ResourceTypePMG,
		ResourceTypeCeph:
		return RelRunsOn
	case ResourceTypeStorage, ResourceTypePhysicalDisk, ResourceTypeNetworkShare:
		return RelMountedTo
	case ResourceTypeAgent, ResourceTypeK8sNode, ResourceTypePod, ResourceTypeK8sDeployment:
		return RelOwnedBy
	default:
		return RelOwnedBy
	}
}
