package registry

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/yangjj-iso/skill-cloud/server/internal/models"
)

// InMemory is a goroutine-safe in-memory Registry, scoped by org.
type InMemory struct {
	mu     sync.RWMutex
	skills map[uuid.UUID]map[string]models.SkillManifest
}

// NewInMemory constructs an empty in-memory registry.
func NewInMemory() *InMemory {
	return &InMemory{skills: map[uuid.UUID]map[string]models.SkillManifest{}}
}

// Upsert stores the manifest for (orgID, namespace, name).
func (r *InMemory) Upsert(_ context.Context, orgID uuid.UUID, m models.SkillManifest) (models.SkillManifest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	bucket, ok := r.skills[orgID]
	if !ok {
		bucket = map[string]models.SkillManifest{}
		r.skills[orgID] = bucket
	}
	bucket[m.QualifiedName()] = m
	return m, nil
}

// Get returns the manifest for a skill within the given org.
func (r *InMemory) Get(_ context.Context, orgID uuid.UUID, namespace, name string) (models.SkillManifest, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	bucket, ok := r.skills[orgID]
	if !ok {
		return models.SkillManifest{}, false, nil
	}
	m, ok := bucket[namespace+"/"+name]
	return m, ok, nil
}

// List returns every skill in the given org.
func (r *InMemory) List(_ context.Context, orgID uuid.UUID) ([]models.SkillManifest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	bucket, ok := r.skills[orgID]
	if !ok {
		return []models.SkillManifest{}, nil
	}
	out := make([]models.SkillManifest, 0, len(bucket))
	for _, m := range bucket {
		out = append(out, m)
	}
	return out, nil
}
