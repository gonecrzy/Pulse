package unifiedresources

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rcourtman/pulse-go-rewrite/internal/models"
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
			key := RelationshipPresentationKey(copy)
			current, exists := merged[key]
			if !exists || !copy.LastSeenAt.Before(current.LastSeenAt) {
				if exists && (current.ObservedAt.IsZero() || (!copy.ObservedAt.IsZero() && current.ObservedAt.Before(copy.ObservedAt))) {
					copy.ObservedAt = current.ObservedAt
				}
				merged[key] = copy
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
		return RelationshipPresentationKey(out[i]) < RelationshipPresentationKey(out[j])
	})
	EnsureUniqueResourceRelationshipIDs(out)
	return out
}

// EnsureUniqueResourceRelationshipIDs disambiguates legacy IDs when distinct
// owner observations share the old endpoint/type/discoverer hash. It preserves
// every already-unique ID, so existing API references remain stable.
func EnsureUniqueResourceRelationshipIDs(relationships []ResourceRelationship) {
	seen := make(map[string]struct{}, len(relationships))
	for index := range relationships {
		relationship := &relationships[index]
		if relationship.ID == "" {
			continue
		}
		if _, exists := seen[relationship.ID]; !exists {
			seen[relationship.ID] = struct{}{}
			continue
		}
		material := RelationshipPresentationKey(*relationship)
		for suffix := 0; ; suffix++ {
			candidate := relationshipIDFromMaterial(material)
			if suffix > 0 {
				candidate = relationshipIDFromMaterial(material + "\x00" + strconv.Itoa(suffix))
			}
			if _, exists := seen[candidate]; !exists {
				relationship.ID = candidate
				seen[candidate] = struct{}{}
				break
			}
		}
	}
}

func relationshipIDFromMaterial(material string) string {
	sum := sha256.Sum256([]byte(material))
	return "relationship_" + hex.EncodeToString(sum[:16])
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
	RelRoutesTo    RelationshipType = "routes_to"
)

// ResourceRelationship represents a typed relationship edge between two unified resources.
type ResourceRelationship struct {
	ID         string           `json:"id,omitempty"`
	SourceID   string           `json:"sourceId"`
	TargetID   string           `json:"targetId"`
	Type       RelationshipType `json:"type"`
	Confidence float64          `json:"confidence"` // 1.0 for defined relationships, lower for heuristics
	EvidenceID string           `json:"evidenceId,omitempty"`
	// EvidenceSource identifies the provider that observed a discovered edge.
	// EvidenceFreshness is updated with that provider's registry freshness; an
	// operator-authored edge is independent of provider polling.
	EvidenceSource    DataSource `json:"evidenceSource,omitempty"`
	EvidenceFreshness string     `json:"evidenceFreshness,omitempty"`
	Reason            string     `json:"reason,omitempty"`
	CreatedBy         string     `json:"createdBy,omitempty"`

	// Provenance and time boundaries
	Active     bool           `json:"active"`     // False means historical edge
	Discoverer string         `json:"discoverer"` // e.g. "docker_adapter" or "pulse_inference_engine"
	ObservedAt time.Time      `json:"observedAt"`
	LastSeenAt time.Time      `json:"lastSeenAt"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// RelationshipPresentationKey identifies one canonical edge and its owner for
// read-only projections. Internal relationship IDs can differ for duplicate
// records with the same meaning, while provider evidence IDs and discoverers
// distinguish legitimate parallel observations. This key never mutates or
// deletes persisted relationships.
func RelationshipPresentationKey(relationship ResourceRelationship) string {
	return strings.Join([]string{
		CanonicalResourceID(relationship.SourceID),
		CanonicalResourceID(relationship.TargetID),
		string(relationship.Type),
		strings.TrimSpace(relationship.Discoverer),
		strings.TrimSpace(string(relationship.EvidenceSource)),
		strings.TrimSpace(relationship.EvidenceID),
		strings.TrimSpace(relationship.CreatedBy),
	}, "\x00")
}

// ResourceRelationshipScope classifies one edge relative to a traversal root.
func ResourceRelationshipScope(rootID string, relationship ResourceRelationship) string {
	rootID = CanonicalResourceID(rootID)
	if CanonicalResourceID(relationship.SourceID) == rootID || CanonicalResourceID(relationship.TargetID) == rootID {
		return "direct"
	}
	return "indirect"
}

// ClassifyResourceRelationshipContext separates service edges, verified
// backing evidence, shared-host paths, and incidental graph neighbors.
func ClassifyResourceRelationshipContext(
	rootID string,
	relationship ResourceRelationship,
	path []string,
	resources map[string]Resource,
) string {
	if relationship.Type == RelStoresOn {
		return "verified_hardware_backing"
	}
	if relationship.Type == RelChecks {
		return "service_monitoring"
	}
	switch relationship.Type {
	case RelRoutesTo, RelExposedBy, RelAttachedTo, RelProtectedBy:
		return "cross_branch_relationship"
	case RelDependsOn:
		if ResourceRelationshipScope(rootID, relationship) == "direct" {
			return "direct_dependency"
		}
		return "indirect_dependency"
	}
	if ResourceRelationshipScope(rootID, relationship) == "direct" {
		return "direct_relationship"
	}
	if RelationshipPathCrossesProxmoxHost(path, resources) {
		return "shared_infrastructure"
	}
	return "indirect_graph_neighbor"
}

// AssessResourceRelationshipImpact describes only what the topology supports.
// It never turns a reachable node or an observed failure into confirmed impact.
func AssessResourceRelationshipImpact(
	rootID string,
	relationship ResourceRelationship,
	path []string,
	resources map[string]Resource,
) string {
	switch ClassifyResourceRelationshipContext(rootID, relationship, path, resources) {
	case "shared_infrastructure":
		return "shared_failure_domain_candidate"
	case "service_monitoring":
		return "service_observation"
	case "direct_dependency", "verified_hardware_backing", "direct_relationship", "indirect_dependency", "cross_branch_relationship":
		return "potential_only"
	default:
		return "not_inferred"
	}
}

// RelationshipPathLess provides stable shortest-path selection for graph
// projections that can reach the same edge through multiple branches.
func RelationshipPathLess(left, right []string) bool {
	if len(left) != len(right) {
		return len(left) < len(right)
	}
	return strings.Join(left, "\x00") < strings.Join(right, "\x00")
}

// RelationshipPathCrossesProxmoxHost reports whether a traversal path crosses
// a canonical Proxmox physical host. A VM is not itself a physical host.
func RelationshipPathCrossesProxmoxHost(path []string, resources map[string]Resource) bool {
	if len(path) < 3 {
		return false
	}
	for _, id := range path[1 : len(path)-1] {
		resource, ok := resources[CanonicalResourceID(id)]
		if !ok || CanonicalResourceType(resource.Type) != ResourceTypeAgent || resource.Proxmox == nil {
			continue
		}
		for _, source := range resource.Sources {
			if source == SourceProxmox {
				return true
			}
		}
	}
	return false
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
	var parentEvidenceSources []DataSource
	for source, sourceParentID := range resource.parentBySource {
		if CanonicalResourceID(sourceParentID) == parentID {
			parentEvidenceSources = append(parentEvidenceSources, source)
		}
	}
	if len(parentEvidenceSources) == 0 && len(resource.Sources) == 1 {
		parentEvidenceSources = append(parentEvidenceSources, resource.Sources[0])
	}
	if len(parentEvidenceSources) == 1 {
		parent.EvidenceSource = parentEvidenceSources[0]
		if sighting, ok := resource.SourceStatus[parent.EvidenceSource]; ok {
			if sighting.Status == "stale" {
				parent.EvidenceFreshness = RelationshipFreshnessStale
			} else if sighting.Status != "unknown" {
				parent.EvidenceFreshness = RelationshipFreshnessCurrent
			}
		}
	}
	normalizeResourceRelationship(&parent)
	return append(relationships, parent)
}

// ProxmoxVMStorageBackingRelationships projects provider-verified QEMU config
// volume IDs onto their exact PVE storage resource. A matching display label
// is insufficient: instance scope and node/shared-storage ownership must also
// match. Physical disks are intentionally not inferred as backing devices.
func ProxmoxVMStorageBackingRelationships(resources []Resource) []ResourceRelationship {
	type storageCandidate struct {
		resource Resource
	}
	storagesByScopeAndName := make(map[string][]storageCandidate)
	for _, resource := range resources {
		if CanonicalResourceType(resource.Type) != ResourceTypeStorage || resource.Proxmox == nil || resource.Storage == nil ||
			!hasDataSource(resource.Sources, SourceProxmox) {
			continue
		}
		instance := strings.TrimSpace(resource.Proxmox.Instance)
		name := strings.TrimSpace(resource.Name)
		if instance == "" || name == "" {
			continue
		}
		key := instance + "\x00" + name
		storagesByScopeAndName[key] = append(storagesByScopeAndName[key], storageCandidate{resource: resource})
	}

	relationships := make([]ResourceRelationship, 0)
	relationshipIndex := make(map[string]int)
	for _, vm := range resources {
		if CanonicalResourceType(vm.Type) != ResourceTypeVM || vm.Proxmox == nil ||
			!hasDataSource(vm.Sources, SourceProxmox) || vm.Proxmox.VMID == 0 || vm.Proxmox.VMConfigObservedAt.IsZero() {
			continue
		}
		instance := strings.TrimSpace(vm.Proxmox.Instance)
		node := strings.TrimSpace(vm.Proxmox.NodeName)
		if instance == "" {
			continue
		}
		for _, volume := range vm.Proxmox.VMConfigStorageVolumes {
			storageID := strings.TrimSpace(volume.StorageID)
			if storageID == "" || strings.TrimSpace(volume.VolumeID) == "" || strings.TrimSpace(volume.DeviceKey) == "" {
				continue
			}
			candidates := storagesByScopeAndName[instance+"\x00"+storageID]
			var nodeMatches []string
			var sharedMatches []string
			for _, candidate := range candidates {
				storage := candidate.resource
				id := CanonicalResourceID(storage.ID)
				if id == "" {
					continue
				}
				if node != "" && strings.EqualFold(strings.TrimSpace(storage.Proxmox.NodeName), node) {
					nodeMatches = append(nodeMatches, id)
					continue
				}
				if storage.Storage.Shared &&
					(strings.EqualFold(strings.TrimSpace(storage.Proxmox.NodeName), "cluster") ||
						len(storage.Storage.Nodes) == 0 || containsFoldedString(storage.Storage.Nodes, node)) {
					sharedMatches = append(sharedMatches, id)
				}
			}
			matches := uniqueStrings(nodeMatches)
			if len(matches) == 0 {
				matches = uniqueStrings(sharedMatches)
			}
			// Ambiguous provider rows must not become an invented association.
			if len(matches) != 1 {
				continue
			}
			relationship := ResourceRelationship{
				SourceID:       vm.ID,
				TargetID:       matches[0],
				Type:           RelStoresOn,
				Confidence:     1,
				EvidenceSource: SourceProxmox,
				Active:         true,
				Discoverer:     "proxmox_vm_config",
				ObservedAt:     vm.Proxmox.VMConfigObservedAt,
				LastSeenAt:     vm.Proxmox.VMConfigObservedAt,
				Metadata: map[string]any{
					"configVolumes": []models.ProxmoxVMStorageVolume{volume},
				},
			}
			if vm.Proxmox.VMConfigObservationStatus == "unavailable" {
				relationship.EvidenceFreshness = RelationshipFreshnessStale
			} else if sourceStatus, ok := vm.SourceStatus[SourceProxmox]; ok {
				if sourceStatus.Status == "stale" || sourceStatus.Status == "offline" {
					relationship.EvidenceFreshness = RelationshipFreshnessStale
				} else if sourceStatus.Status != "unknown" {
					relationship.EvidenceFreshness = RelationshipFreshnessCurrent
				}
			}
			normalizeResourceRelationship(&relationship)
			key := RelationshipPresentationKey(relationship)
			if index, exists := relationshipIndex[key]; exists {
				volumes, _ := relationships[index].Metadata["configVolumes"].([]models.ProxmoxVMStorageVolume)
				alreadyIncluded := false
				for _, existing := range volumes {
					if existing == volume {
						alreadyIncluded = true
						break
					}
				}
				if !alreadyIncluded {
					volumes = append(volumes, volume)
					sort.Slice(volumes, func(i, j int) bool {
						if volumes[i].DeviceKey != volumes[j].DeviceKey {
							return volumes[i].DeviceKey < volumes[j].DeviceKey
						}
						return volumes[i].VolumeID < volumes[j].VolumeID
					})
					relationships[index].Metadata["configVolumes"] = volumes
				}
				continue
			}
			relationshipIndex[key] = len(relationships)
			relationships = append(relationships, relationship)
		}
	}
	return mergeResourceRelationships(nil, relationships)
}

func containsFoldedString(values []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), target) {
			return true
		}
	}
	return false
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
	relationship.EvidenceSource = DataSource(strings.TrimSpace(string(relationship.EvidenceSource)))
	relationship.EvidenceFreshness = strings.TrimSpace(relationship.EvidenceFreshness)
	relationship.Reason = strings.TrimSpace(relationship.Reason)
	relationship.CreatedBy = strings.TrimSpace(relationship.CreatedBy)
	relationship.EvidenceID = strings.TrimSpace(relationship.EvidenceID)
	if strings.TrimSpace(relationship.ID) != "" ||
		relationship.SourceID == "" ||
		relationship.TargetID == "" ||
		relationship.Type == "" {
		return
	}
	// Keep the long-standing relationship ID recipe stable. Presentation
	// deduplication uses RelationshipPresentationKey, which also includes owner
	// provenance, but adding those fields here would rewrite persisted canonical
	// IDs and break existing API references.
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
