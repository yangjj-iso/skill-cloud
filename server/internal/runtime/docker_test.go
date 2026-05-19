package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yangjj-iso/skill-cloud/server/internal/models"
)

func newFakeDocker(t *testing.T, hook func(ctx context.Context, name string, args []string, stdin []byte) ([]byte, []byte, int, error)) *Docker {
	t.Helper()
	// Bypass exec.LookPath in NewDocker — we construct the runner
	// directly so unit tests don't need a real `docker` binary on the
	// box CI image.
	return &Docker{binary: "docker-fake", runCmd: hook}
}

func dockerManifest() models.SkillManifest {
	return models.SkillManifest{
		Namespace: "acme",
		Name:      "hello",
		Version:   "0.1.0",
		Runtime: models.Runtime{
			Type:           models.RuntimeDocker,
			Image:          "alpine:3.20",
			TimeoutSeconds: 5,
			MemoryMB:       128,
		},
	}
}

func TestDockerRunHappyPath(t *testing.T) {
	var seenArgs []string
	var seenStdin []byte
	d := newFakeDocker(t, func(_ context.Context, _ string, args []string, stdin []byte) ([]byte, []byte, int, error) {
		seenArgs = args
		seenStdin = stdin
		return []byte(`{"greeting":"hi"}`), nil, 0, nil
	})

	res, err := d.Run(context.Background(), Request{
		Skill: dockerManifest(),
		Input: map[string]any{"name": "world"},
	})
	require.NoError(t, err)
	assert.Equal(t, StatusOK, res.Status)
	assert.Equal(t, "hi", res.Output["greeting"])
	assert.Contains(t, seenArgs, "--rm")
	assert.Contains(t, seenArgs, "--network=none")
	assert.Contains(t, seenArgs, "--read-only")
	assert.Contains(t, seenArgs, "ALL")
	assert.Contains(t, seenArgs, "no-new-privileges")
	assert.Contains(t, seenArgs, "alpine:3.20")
	assert.Contains(t, string(seenStdin), `"world"`)
}

func TestDockerRunMultiWordEntrypoint(t *testing.T) {
	// Regression: `entrypoint: "python -m hello"` must turn into
	//   --entrypoint python <image> -m hello
	// not
	//   --entrypoint "python -m hello" <image>
	var seenArgs []string
	d := newFakeDocker(t, func(_ context.Context, _ string, args []string, _ []byte) ([]byte, []byte, int, error) {
		seenArgs = args
		return []byte(`{}`), nil, 0, nil
	})
	skill := dockerManifest()
	skill.Runtime.Entrypoint = "python -m hello"

	res, err := d.Run(context.Background(), Request{Skill: skill})
	require.NoError(t, err)
	require.Equal(t, StatusOK, res.Status)

	// Locate --entrypoint and assert it gets exactly one token, then
	// the image, then the remaining argv tail.
	idx := -1
	for i, a := range seenArgs {
		if a == "--entrypoint" {
			idx = i
			break
		}
	}
	require.NotEqual(t, -1, idx, "--entrypoint must be present")
	assert.Equal(t, "python", seenArgs[idx+1], "--entrypoint receives only the executable")
	// The image must come after the --entrypoint pair and before the
	// remaining tokens.
	imageIdx := idx + 2
	assert.Equal(t, "alpine:3.20", seenArgs[imageIdx], "image must come right after --entrypoint <exec>")
	assert.Equal(t, []string{"-m", "hello"}, seenArgs[imageIdx+1:], "remaining tokens become CMD args after image")
}

func TestDockerRunNonZeroExitBecomesError(t *testing.T) {
	d := newFakeDocker(t, func(_ context.Context, _ string, _ []string, _ []byte) ([]byte, []byte, int, error) {
		return nil, []byte("boom\n"), 1, nil
	})
	res, err := d.Run(context.Background(), Request{Skill: dockerManifest()})
	require.NoError(t, err)
	assert.Equal(t, StatusError, res.Status)
	assert.Contains(t, res.ErrorMessage, "exited 1")
	assert.Contains(t, res.ErrorMessage, "boom")
}

func TestDockerRunTimeoutClassified(t *testing.T) {
	d := newFakeDocker(t, func(ctx context.Context, _ string, _ []string, _ []byte) ([]byte, []byte, int, error) {
		<-ctx.Done()
		return nil, nil, -1, ctx.Err()
	})
	skill := dockerManifest()
	skill.Runtime.TimeoutSeconds = 1
	res, err := d.Run(context.Background(), Request{Skill: skill})
	require.NoError(t, err)
	assert.Equal(t, StatusTimeout, res.Status)
}

func TestDockerRunRejectsNonJSONStdout(t *testing.T) {
	d := newFakeDocker(t, func(_ context.Context, _ string, _ []string, _ []byte) ([]byte, []byte, int, error) {
		return []byte("not json"), nil, 0, nil
	})
	res, err := d.Run(context.Background(), Request{Skill: dockerManifest()})
	require.NoError(t, err)
	assert.Equal(t, StatusError, res.Status)
	assert.Contains(t, res.ErrorMessage, "non-JSON")
}

func TestDockerRunRejectsEmptyImage(t *testing.T) {
	d := newFakeDocker(t, func(_ context.Context, _ string, _ []string, _ []byte) ([]byte, []byte, int, error) {
		t.Fatal("docker should not be invoked when image is empty")
		return nil, nil, 0, nil
	})
	skill := dockerManifest()
	skill.Runtime.Image = ""
	res, err := d.Run(context.Background(), Request{Skill: skill})
	require.Error(t, err)
	assert.Equal(t, StatusError, res.Status)
}

func TestDockerRunOverlongStdout(t *testing.T) {
	huge := strings.Repeat("a", MaxOutputBytes+10)
	d := newFakeDocker(t, func(_ context.Context, _ string, _ []string, _ []byte) ([]byte, []byte, int, error) {
		return []byte(`{"blob":"` + huge + `"}`), nil, 0, nil
	})
	res, err := d.Run(context.Background(), Request{Skill: dockerManifest()})
	require.NoError(t, err)
	assert.Equal(t, StatusError, res.Status)
	assert.Contains(t, res.ErrorMessage, "exceeded")
}
