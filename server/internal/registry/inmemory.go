// Package registry stores skill manifests. The in-memory implementation
// here is suitable for early development and tests; a Postgres-backed
// implementation will replace it before MVP.
package registry

import (
	"sync"

	"github.com/yangjj-iso/skill-cloud/server/internal/models"
)

// InMemory is a goroutine-safe in-memory skill registry.
type InMemory struct {
	mu     sync.RWMutex
	skills map[string]models.SkillManifest
}

// NewInMemory constructs an empty in-memory registry.
func NewInMemory() *InMemory {
	return &InMemory{skills: map[string]models.SkillManifest{}}
}

// Upsert inserts or replaces a skill in the registry, keyed by qualified name.
func (r *InMemory) Upsert(m models.SkillManifest) models.SkillManifest {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.skills[m.QualifiedName()] = m
	return m
}

// Get returns the manifest for a skill or false if it does not exist.
func (r *InMemory) Get(namespace, name string) (models.SkillManifest, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.skills[namespace+"/"+name]
	return m, ok
}

// List returns every registered skill, in an unspecified order.
func (r *InMemory) List() []models.SkillManifest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]models.SkillManifest, 0, len(r.skills))
	for _, m := range r.skills {
		out = append(out, m)
	}
	return out
}
