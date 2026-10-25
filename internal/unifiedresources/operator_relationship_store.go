package unifiedresources

import (
	"fmt"
	"time"
)

func (s *SQLiteResourceStore) PutOperatorRelationship(relationship OperatorResourceRelationship) error {
	if err := normalizeOperatorResourceRelationship(&relationship); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`
		INSERT INTO operator_resource_relationships
			(id, source_id, target_id, relationship_type, reason, created_by, updated_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			reason=excluded.reason,
			updated_by=excluded.updated_by,
			updated_at=excluded.updated_at,
			removed_by='',
			removed_at=NULL
	`, relationship.ID, relationship.SourceID, relationship.TargetID, relationship.Type,
		relationship.Reason, relationship.CreatedBy, relationship.UpdatedBy,
		relationship.CreatedAt.UTC(), relationship.UpdatedAt.UTC())
	if err != nil {
		return fmt.Errorf("store operator relationship: %w", err)
	}
	return nil
}

func (s *SQLiteResourceStore) RemoveOperatorRelationship(sourceID, targetID string, relationshipType RelationshipType, actor string, removedAt time.Time) (bool, error) {
	relationship := OperatorResourceRelationship{SourceID: sourceID, TargetID: targetID, Type: relationshipType}
	if err := normalizeOperatorResourceRelationship(&relationship); err != nil {
		return false, err
	}
	if removedAt.IsZero() {
		removedAt = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := s.db.Exec(`
		UPDATE operator_resource_relationships
		SET removed_by=?, removed_at=?, updated_by=?, updated_at=?
		WHERE id=? AND removed_at IS NULL
	`, actor, removedAt.UTC(), actor, removedAt.UTC(), relationship.ID)
	if err != nil {
		return false, fmt.Errorf("remove operator relationship: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read removed operator relationship count: %w", err)
	}
	return rows > 0, nil
}

func (s *SQLiteResourceStore) ListOperatorRelationships() ([]OperatorResourceRelationship, error) {
	rows, err := s.db.Query(`
		SELECT id, source_id, target_id, relationship_type, reason, created_by, updated_by, created_at, updated_at
		FROM operator_resource_relationships
		WHERE removed_at IS NULL
		ORDER BY source_id, target_id, relationship_type
	`)
	if err != nil {
		return nil, fmt.Errorf("query operator relationships: %w", err)
	}
	defer rows.Close()
	var relationships []OperatorResourceRelationship
	for rows.Next() {
		var relationship OperatorResourceRelationship
		if err := rows.Scan(
			&relationship.ID,
			&relationship.SourceID,
			&relationship.TargetID,
			&relationship.Type,
			&relationship.Reason,
			&relationship.CreatedBy,
			&relationship.UpdatedBy,
			&relationship.CreatedAt,
			&relationship.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan operator relationship: %w", err)
		}
		relationships = append(relationships, relationship)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate operator relationships: %w", err)
	}
	return relationships, nil
}

func (m *MemoryStore) PutOperatorRelationship(relationship OperatorResourceRelationship) error {
	if err := normalizeOperatorResourceRelationship(&relationship); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if previous, exists := m.operatorRelationships[relationship.ID]; exists {
		relationship.CreatedBy = previous.CreatedBy
		relationship.CreatedAt = previous.CreatedAt
	}
	relationship.RemovedAt = time.Time{}
	relationship.RemovedBy = ""
	m.operatorRelationships[relationship.ID] = relationship
	return nil
}

func (m *MemoryStore) RemoveOperatorRelationship(sourceID, targetID string, relationshipType RelationshipType, actor string, removedAt time.Time) (bool, error) {
	relationship := OperatorResourceRelationship{SourceID: sourceID, TargetID: targetID, Type: relationshipType}
	if err := normalizeOperatorResourceRelationship(&relationship); err != nil {
		return false, err
	}
	if removedAt.IsZero() {
		removedAt = time.Now().UTC()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	stored, exists := m.operatorRelationships[relationship.ID]
	if !exists || !stored.RemovedAt.IsZero() {
		return false, nil
	}
	stored.RemovedAt = removedAt.UTC()
	stored.RemovedBy = actor
	stored.UpdatedAt = removedAt.UTC()
	stored.UpdatedBy = actor
	m.operatorRelationships[relationship.ID] = stored
	return true, nil
}

func (m *MemoryStore) ListOperatorRelationships() ([]OperatorResourceRelationship, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var relationships []OperatorResourceRelationship
	for _, relationship := range m.operatorRelationships {
		if relationship.RemovedAt.IsZero() {
			relationships = append(relationships, relationship)
		}
	}
	return relationships, nil
}
