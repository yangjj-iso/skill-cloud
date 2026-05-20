package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yangjj-iso/skill-cloud/server/internal/models"
)

// fakeRunner records the request it was handed so tests can assert
// that the dispatcher routed correctly and applied resource defaults.
type fakeRunner struct {
	called bool
	got    Request
	result Result
}

func (f *fakeRunner) Run(_ context.Context, req Request) (Result, error) {
	f.called = true
	f.got = req
	return f.result, nil
}

func TestDispatcherRoutesByRuntimeType(t *testing.T) {
	docker := &fakeRunner{result: Result{Status: StatusOK, Output: map[string]any{"r": "docker"}}}
	httpR := &fakeRunner{result: Result{Status: StatusOK, Output: map[string]any{"r": "http"}}}
	d := NewDispatcher(docker, httpR)

	out, err := d.Run(context.Background(), Request{
		Skill: models.SkillManifest{Runtime: models.Runtime{Type: models.RuntimeDocker, Image: "x"}},
	})
	require.NoError(t, err)
	assert.True(t, docker.called)
	assert.False(t, httpR.called)
	assert.Equal(t, "docker", out.Output["r"])

	docker.called = false
	_, err = d.Run(context.Background(), Request{
		Skill: models.SkillManifest{Runtime: models.Runtime{Type: models.RuntimeHTTPProxy, URL: "http://x"}},
	})
	require.NoError(t, err)
	assert.False(t, docker.called)
	assert.True(t, httpR.called)
}

func TestDispatcherAppliesDefaults(t *testing.T) {
	r := &fakeRunner{result: Result{Status: StatusOK}}
	d := NewDispatcher(r, r)
	_, _ = d.Run(context.Background(), Request{
		Skill: models.SkillManifest{Runtime: models.Runtime{Type: models.RuntimeDocker, Image: "x"}},
	})
	require.True(t, r.called)
	assert.Equal(t, DefaultTimeoutSeconds, r.got.Skill.Runtime.TimeoutSeconds)
	assert.Equal(t, DefaultMemoryMB, r.got.Skill.Runtime.MemoryMB)
}

func TestDispatcherUnavailableRuntime(t *testing.T) {
	d := NewDispatcher(nil, nil)
	res, err := d.Run(context.Background(), Request{
		Skill: models.SkillManifest{Runtime: models.Runtime{Type: models.RuntimeDocker, Image: "x"}},
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRuntimeUnavailable))
	assert.Equal(t, StatusError, res.Status)
	assert.Contains(t, res.ErrorMessage, "docker")
}

func TestDispatcherUnknownType(t *testing.T) {
	d := NewDispatcher(&fakeRunner{}, &fakeRunner{})
	res, err := d.Run(context.Background(), Request{
		Skill: models.SkillManifest{Runtime: models.Runtime{Type: "wat"}},
	})
	require.Error(t, err)
	assert.Equal(t, StatusError, res.Status)
}
