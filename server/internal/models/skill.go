// Package models defines the core domain types for Skill Cloud.
package models

import (
	"errors"
	"fmt"
	"regexp"
)

// RuntimeType identifies how a skill is executed.
type RuntimeType string

const (
	// RuntimeDocker means the platform runs the skill inside a Docker
	// container using the image referenced in the manifest.
	RuntimeDocker RuntimeType = "docker"
	// RuntimeHTTPProxy means the platform forwards the invocation to
	// an externally hosted HTTP endpoint.
	RuntimeHTTPProxy RuntimeType = "http_proxy"
)

// Runtime describes how a skill is executed.
type Runtime struct {
	Type           RuntimeType `json:"type" yaml:"type"`
	Image          string      `json:"image,omitempty" yaml:"image,omitempty"`
	Entrypoint     string      `json:"entrypoint,omitempty" yaml:"entrypoint,omitempty"`
	URL            string      `json:"url,omitempty" yaml:"url,omitempty"`
	TimeoutSeconds int         `json:"timeout_seconds,omitempty" yaml:"timeout_seconds,omitempty"`
	MemoryMB       int         `json:"memory_mb,omitempty" yaml:"memory_mb,omitempty"`
}

// IOField describes one entry in the inputs or outputs schema.
type IOField struct {
	Type        string `json:"type" yaml:"type"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Required    bool   `json:"required,omitempty" yaml:"required,omitempty"`
	Default     any    `json:"default,omitempty" yaml:"default,omitempty"`
}

// SkillManifest is the canonical description of a skill version.
type SkillManifest struct {
	Name        string             `json:"name" yaml:"name"`
	Namespace   string             `json:"namespace" yaml:"namespace"`
	Version     string             `json:"version" yaml:"version"`
	Description string             `json:"description,omitempty" yaml:"description,omitempty"`
	Tags        []string           `json:"tags,omitempty" yaml:"tags,omitempty"`
	Runtime     Runtime            `json:"runtime" yaml:"runtime"`
	Inputs      map[string]IOField `json:"inputs,omitempty" yaml:"inputs,omitempty"`
	Outputs     map[string]IOField `json:"outputs,omitempty" yaml:"outputs,omitempty"`
}

// QualifiedName returns the fully-qualified skill name `<namespace>/<name>`.
func (m SkillManifest) QualifiedName() string {
	return fmt.Sprintf("%s/%s", m.Namespace, m.Name)
}

// Redacted returns the manifest with runtime implementation details
// stripped: image / entrypoint / url are blanked while the runtime
// `type` and resource limits are kept (callers still need to know
// whether a skill is docker-backed or proxied, and how slow / hungry it
// may be). This is the projection returned from public-facing list/get
// endpoints and the MCP tools/list response so anyone discovering a
// skill cannot trivially copy the underlying implementation.
//
// Owners fetch the full manifest (including runtime internals) via the
// dedicated /v1/skills/:namespace/:name/runtime endpoint.
func (m SkillManifest) Redacted() SkillManifest {
	r := m
	r.Runtime = Runtime{
		Type:           m.Runtime.Type,
		TimeoutSeconds: m.Runtime.TimeoutSeconds,
		MemoryMB:       m.Runtime.MemoryMB,
	}
	return r
}

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)
var semverRe = regexp.MustCompile(`^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

// Validate checks that the manifest is well-formed.
func (m SkillManifest) Validate() error {
	if !nameRe.MatchString(m.Namespace) {
		return errors.New("invalid namespace (must match ^[a-z0-9][a-z0-9_-]{0,62}$)")
	}
	if !nameRe.MatchString(m.Name) {
		return errors.New("invalid name (must match ^[a-z0-9][a-z0-9_-]{0,62}$)")
	}
	if !semverRe.MatchString(m.Version) {
		return errors.New("invalid version (must be semver, e.g. 1.2.3)")
	}
	switch m.Runtime.Type {
	case RuntimeDocker:
		if m.Runtime.Image == "" {
			return errors.New("runtime.image is required for docker runtime")
		}
	case RuntimeHTTPProxy:
		if m.Runtime.URL == "" {
			return errors.New("runtime.url is required for http_proxy runtime")
		}
	default:
		return fmt.Errorf("unknown runtime.type: %q (expected docker or http_proxy)", m.Runtime.Type)
	}
	return nil
}
