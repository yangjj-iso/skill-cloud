package models_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/yangjj-iso/skill-cloud/server/internal/models"
)

func TestSkillManifestValidate(t *testing.T) {
	cases := []struct {
		name    string
		m       models.SkillManifest
		wantErr bool
	}{
		{
			name: "valid docker",
			m: models.SkillManifest{
				Namespace: "acme",
				Name:      "hello",
				Version:   "1.0.0",
				Runtime:   models.Runtime{Type: models.RuntimeDocker, Image: "python:3.12"},
			},
		},
		{
			name: "valid http_proxy",
			m: models.SkillManifest{
				Namespace: "acme",
				Name:      "hello",
				Version:   "1.0.0",
				Runtime:   models.Runtime{Type: models.RuntimeHTTPProxy, URL: "https://example.com/skill"},
			},
		},
		{
			name: "bad namespace",
			m: models.SkillManifest{
				Namespace: "Acme",
				Name:      "hello",
				Version:   "1.0.0",
				Runtime:   models.Runtime{Type: models.RuntimeDocker, Image: "x"},
			},
			wantErr: true,
		},
		{
			name: "bad semver",
			m: models.SkillManifest{
				Namespace: "acme",
				Name:      "hello",
				Version:   "v1",
				Runtime:   models.Runtime{Type: models.RuntimeDocker, Image: "x"},
			},
			wantErr: true,
		},
		{
			name: "semver with prerelease",
			m: models.SkillManifest{
				Namespace: "acme",
				Name:      "hello",
				Version:   "1.0.0-alpha",
				Runtime:   models.Runtime{Type: models.RuntimeDocker, Image: "x"},
			},
		},
		{
			name: "semver with build metadata",
			m: models.SkillManifest{
				Namespace: "acme",
				Name:      "hello",
				Version:   "1.0.0+build.1",
				Runtime:   models.Runtime{Type: models.RuntimeDocker, Image: "x"},
			},
		},
		{
			name: "semver with prerelease and build metadata",
			m: models.SkillManifest{
				Namespace: "acme",
				Name:      "hello",
				Version:   "1.0.0-alpha+001",
				Runtime:   models.Runtime{Type: models.RuntimeDocker, Image: "x"},
			},
		},
		{
			name: "semver with dotted prerelease and build metadata",
			m: models.SkillManifest{
				Namespace: "acme",
				Name:      "hello",
				Version:   "1.0.0-beta.1+build.123",
				Runtime:   models.Runtime{Type: models.RuntimeDocker, Image: "x"},
			},
		},
		{
			name: "docker missing image",
			m: models.SkillManifest{
				Namespace: "acme",
				Name:      "hello",
				Version:   "1.0.0",
				Runtime:   models.Runtime{Type: models.RuntimeDocker},
			},
			wantErr: true,
		},
		{
			name: "http_proxy missing url",
			m: models.SkillManifest{
				Namespace: "acme",
				Name:      "hello",
				Version:   "1.0.0",
				Runtime:   models.Runtime{Type: models.RuntimeHTTPProxy},
			},
			wantErr: true,
		},
		{
			name: "unknown runtime type",
			m: models.SkillManifest{
				Namespace: "acme",
				Name:      "hello",
				Version:   "1.0.0",
				Runtime:   models.Runtime{Type: "wasm"},
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.m.Validate()
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestQualifiedName(t *testing.T) {
	m := models.SkillManifest{Namespace: "acme", Name: "hello"}
	assert.Equal(t, "acme/hello", m.QualifiedName())
}
