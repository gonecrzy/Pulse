package unifiedresources

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"
)

const operatorRelationshipDiscoverer = "operator_store"

const (
	RelationshipFreshnessCurrent = "current"
	RelationshipFreshnessStale   = "stale"
)

// OperatorResourceRelationship is a durable, org-scoped operator-authored
// relationship. ResourceStore instances are already scoped to one org, so the
// row does not duplicate tenant identity. Removed rows are retained as a small
// audit tombstone and are omitted from the registry projection.
type OperatorResourceRelationship struct {
	ID        string           `json:"id"`
	SourceID  string           `json:"sourceId"`
	TargetID  string           `json:"targetId"`
	Type      RelationshipType `json:"type"`
	Reason    string           `json:"reason,omitempty"`
	CreatedBy string           `json:"createdBy,omitempty"`
	UpdatedBy string           `json:"updatedBy,omitempty"`
	CreatedAt time.Time        `json:"createdAt"`
	UpdatedAt time.Time        `json:"updatedAt"`
	RemovedBy string           `json:"removedBy,omitempty"`
	RemovedAt time.Time        `json:"removedAt,omitempty"`
}

// OperatorRelationshipStore is an optional ResourceStore capability for
// durable operator-authored dependency and routing edges.
type OperatorRelationshipStore interface {
	PutOperatorRelationship(relationship OperatorResourceRelationship) error
	RemoveOperatorRelationship(sourceID, targetID string, relationshipType RelationshipType, actor string, removedAt time.Time) (bool, error)
	ListOperatorRelationships() ([]OperatorResourceRelationship, error)
}

// IsOperatorRelationshipType reports the directed relationship types the
// operator API may persist. Identity equivalence remains ResourceLink state.
func IsOperatorRelationshipType(relationshipType RelationshipType) bool {
	switch relationshipType {
	case RelDependsOn, RelRunsOn, RelHostedBy, RelRoutesTo:
		return true
	default:
		return false
	}
}

func normalizeOperatorResourceRelationship(relationship *OperatorResourceRelationship) error {
	if relationship == nil {
		return fmt.Errorf("relationship is required")
	}
	relationship.SourceID = CanonicalResourceID(relationship.SourceID)
	relationship.TargetID = CanonicalResourceID(relationship.TargetID)
	if relationship.SourceID == "" || relationship.TargetID == "" {
		return fmt.Errorf("source and target resource IDs are required")
	}
	if relationship.SourceID == relationship.TargetID {
		return fmt.Errorf("a resource cannot have a relationship to itself")
	}
	if !IsOperatorRelationshipType(relationship.Type) {
		return fmt.Errorf("unsupported operator relationship type %q", relationship.Type)
	}
	relationship.Reason = strings.TrimSpace(relationship.Reason)
	relationship.CreatedBy = strings.TrimSpace(relationship.CreatedBy)
	relationship.UpdatedBy = strings.TrimSpace(relationship.UpdatedBy)
	relationship.RemovedBy = strings.TrimSpace(relationship.RemovedBy)
	relationship.ID = operatorRelationshipID(relationship.SourceID, relationship.TargetID, relationship.Type)
	if relationship.CreatedAt.IsZero() {
		relationship.CreatedAt = time.Now().UTC()
	}
	if relationship.UpdatedAt.IsZero() {
		relationship.UpdatedAt = relationship.CreatedAt
	}
	return nil
}

func operatorRelationshipID(sourceID, targetID string, relationshipType RelationshipType) string {
	key := string(relationshipType) + "\x00" + CanonicalResourceID(sourceID) + "\x00" + CanonicalResourceID(targetID)
	sum := sha256.Sum256([]byte(key))
	return "operator-rel-" + hex.EncodeToString(sum[:16])
}

// OperatorRelationshipID returns the stable ID for one directed operator edge.
func OperatorRelationshipID(sourceID, targetID string, relationshipType RelationshipType) string {
	return operatorRelationshipID(sourceID, targetID, relationshipType)
}

func (rr *ResourceRegistry) loadOperatorRelationships() {
	if rr.store == nil {
		return
	}
	store, ok := rr.store.(OperatorRelationshipStore)
	if !ok {
		return
	}
	relationships, err := store.ListOperatorRelationships()
	if err != nil {
		log.Printf("unifiedresources: failed to load operator relationships: %v", err)
		rr.operatorRelationshipsUnreadable = true
		return
	}
	rr.operatorRelationships = relationships
}

// ValidateIdentityLink rejects a proposed link when the two current identity
// components contain different native IDs from the same provider. Different
// providers remain linkable, and already-shared native IDs do not conflict.
// Missing resources contribute no evidence and can be checked again when they
// return; this does not infer identity from names.
func (rr *ResourceRegistry) ValidateIdentityLink(resourceA, resourceB string) error {
	resourceA = CanonicalResourceID(resourceA)
	resourceB = CanonicalResourceID(resourceB)
	if resourceA == "" || resourceB == "" {
		return fmt.Errorf("resource IDs are required")
	}
	rr.mu.RLock()
	defer rr.mu.RUnlock()
	memberA, foundA := rr.linkMemberLocked(resourceA)
	memberB, foundB := rr.linkMemberLocked(resourceB)
	if !foundA {
		return &IdentityLinkUnavailableError{ResourceID: resourceA}
	}
	if !foundB {
		return &IdentityLinkUnavailableError{ResourceID: resourceB}
	}
	if memberA.row == memberB.row {
		return nil
	}
	identitiesA := rr.providerIdentitiesForLinkMemberLocked(memberA.row)
	identitiesB := rr.providerIdentitiesForLinkMemberLocked(memberB.row)
	providers := make([]string, 0, len(identitiesA))
	for source := range identitiesA {
		if len(identitiesB[source]) > 0 {
			providers = append(providers, string(source))
		}
	}
	sort.Strings(providers)
	for _, provider := range providers {
		source := DataSource(provider)
		union := make(map[string]struct{}, len(identitiesA[source])+len(identitiesB[source]))
		for id := range identitiesA[source] {
			union[id] = struct{}{}
		}
		for id := range identitiesB[source] {
			union[id] = struct{}{}
		}
		if len(union) <= 1 {
			continue
		}
		ids := make([]string, 0, len(union))
		for id := range union {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		return &IdentityLinkConflictError{Source: source, SourceIDs: ids}
	}
	return nil
}

type IdentityLinkConflictError struct {
	Source    DataSource
	SourceIDs []string
}

type IdentityLinkUnavailableError struct {
	ResourceID string
}

func (e *IdentityLinkUnavailableError) Error() string {
	if e == nil {
		return "identity link cannot be validated because a resource is unavailable"
	}
	return fmt.Sprintf("identity link cannot be validated because resource %s is unavailable", e.ResourceID)
}

func (e *IdentityLinkConflictError) Error() string {
	if e == nil {
		return "identity link conflicts with existing provider identities"
	}
	return fmt.Sprintf("identity link conflicts: %s reports distinct resources %s", e.Source, strings.Join(e.SourceIDs, ", "))
}

func (rr *ResourceRegistry) providerIdentitiesForLinkMemberLocked(member *Resource) map[DataSource]map[string]struct{} {
	identities := make(map[DataSource]map[string]struct{})
	if member == nil {
		return identities
	}
	for source, mappings := range rr.bySource {
		for sourceID, resourceID := range mappings {
			if CanonicalResourceID(resourceID) != member.ID {
				continue
			}
			if identities[source] == nil {
				identities[source] = make(map[string]struct{})
			}
			identities[source][normalizeSourceID(sourceID)] = struct{}{}
		}
	}
	return identities
}

func (rr *ResourceRegistry) operatorRelationshipsForResourceLocked(resource *Resource) {
	if resource == nil || len(rr.operatorRelationships) == 0 {
		return
	}
	for _, saved := range rr.operatorRelationships {
		sourceID := rr.relationshipEndpointLocked(saved.SourceID)
		if sourceID != resource.ID {
			continue
		}
		targetID := rr.relationshipEndpointLocked(saved.TargetID)
		if targetID == resource.ID {
			continue
		}
		relationship := ResourceRelationship{
			ID:                saved.ID,
			SourceID:          sourceID,
			TargetID:          targetID,
			Type:              saved.Type,
			Confidence:        1,
			Active:            true,
			Discoverer:        operatorRelationshipDiscoverer,
			EvidenceFreshness: RelationshipFreshnessCurrent,
			Reason:            saved.Reason,
			CreatedBy:         saved.CreatedBy,
			ObservedAt:        saved.CreatedAt,
			LastSeenAt:        saved.UpdatedAt,
		}
		resource.Relationships = mergeResourceRelationships(resource.Relationships, []ResourceRelationship{relationship})
	}
}

func (rr *ResourceRegistry) relationshipEndpointLocked(resourceID string) string {
	resourceID = CanonicalResourceID(resourceID)
	if rr.resources[resourceID] != nil {
		return resourceID
	}
	if holder, ambiguous := rr.linkFoldHolderLocked(resourceID); holder != "" && !ambiguous {
		return holder
	}
	return resourceID
}

func (rr *ResourceRegistry) refreshRelationshipFreshnessLocked(resource *Resource) bool {
	if resource == nil {
		return false
	}
	changed := false
	for index := range resource.Relationships {
		relationship := &resource.Relationships[index]
		if relationship.Discoverer == operatorRelationshipDiscoverer {
			changed = changed || relationship.EvidenceFreshness != RelationshipFreshnessCurrent
			relationship.EvidenceFreshness = RelationshipFreshnessCurrent
			continue
		}
		source := relationship.EvidenceSource
		if source == "" {
			source = relationshipSourceFromDiscoverer(relationship.Discoverer)
		}
		if source == "" {
			continue
		}
		sighting, ok := resource.SourceStatus[source]
		if !ok {
			continue
		}
		if sighting.Status == "stale" {
			changed = changed || relationship.EvidenceFreshness != RelationshipFreshnessStale
			relationship.EvidenceFreshness = RelationshipFreshnessStale
		} else if sighting.Status != "unknown" {
			changed = changed || relationship.EvidenceFreshness != RelationshipFreshnessCurrent
			relationship.EvidenceFreshness = RelationshipFreshnessCurrent
		}
	}
	return changed
}

func relationshipSourceFromDiscoverer(discoverer string) DataSource {
	switch strings.TrimSpace(discoverer) {
	case "komodo_adapter":
		return SourceKomodo
	case "proxmox_adapter":
		return SourceProxmox
	case dockerAdapterRelationshipDiscoverer:
		return SourceDocker
	case "availability_attachment":
		return SourceAvailability
	case "k8s_adapter":
		return SourceK8s
	default:
		return ""
	}
}
