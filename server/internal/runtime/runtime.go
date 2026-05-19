// Package runtime executes a registered skill. Two implementations are
// shipped:
//
//   - HTTPProxy forwards the invocation to an externally hosted skill at
//     the URL recorded in the manifest. Useful when a team already has
//     a microservice they want to expose as a skill without rehosting
//     it.
//   - Docker spins up a one-shot container per invocation, pipes the
//     input JSON to stdin, and reads the output JSON from stdout. The
//     container is sandboxed with --network=none, --read-only,
//     --cap-drop ALL, --user nobody, and resource caps from the
//     manifest.
//
// A Dispatcher chooses between the two based on the manifest. The api
// package owns the Dispatcher and calls Run() from both the REST
// invoke handler and the MCP tools/call handler so behaviour and
// audit logging are identical regardless of how the call arrives.
package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/yangjj-iso/skill-cloud/server/internal/models"
)

// Default execution caps. The dispatcher applies these whenever the
// manifest omits explicit limits — running a skill without limits is
// never safe.
const (
	DefaultTimeoutSeconds = 30
	DefaultMemoryMB       = 256
	// MaxOutputBytes bounds the size of an individual skill output to
	// keep one runaway invocation from filling memory or the audit log.
	// 1 MiB is plenty for JSON-shaped responses.
	MaxOutputBytes = 1 << 20
)

// Request is what a runner receives. Skill / Input are mandatory;
// Timeout is the effective timeout the dispatcher resolved from the
// manifest (it is never zero — defaults are applied before Run is
// called).
type Request struct {
	Skill models.SkillManifest
	Input map[string]any
}

// Result is what a runner returns. Status is one of `ok`, `error`, or
// `timeout`. ErrorMessage is empty on success. Output is the JSON-y
// response payload (free-form map; the dispatcher does not impose a
// schema beyond what manifest.outputs declares).
type Result struct {
	Status       string
	Output       map[string]any
	ErrorMessage string
	// OutputBytes is the length of the JSON encoding of Output. The
	// invocation log records this directly so callers don't have to
	// recompute it.
	OutputBytes int
}

const (
	StatusOK      = "ok"
	StatusError   = "error"
	StatusTimeout = "timeout"
)

// Runner executes a single invocation.
type Runner interface {
	Run(ctx context.Context, req Request) (Result, error)
}

// Dispatcher selects the right Runner for a manifest and forwards the
// call. It applies default resource limits when the manifest leaves
// them blank.
type Dispatcher struct {
	docker Runner
	http   Runner
}

// NewDispatcher returns a Dispatcher backed by the supplied runners.
// Either runner may be nil — calls that target an absent runtime fail
// with ErrRuntimeUnavailable instead of panicking.
func NewDispatcher(docker, http Runner) *Dispatcher {
	return &Dispatcher{docker: docker, http: http}
}

// ErrRuntimeUnavailable is returned when a skill targets a runtime
// (docker / http_proxy) that the server was started without.
var ErrRuntimeUnavailable = errors.New("runtime: requested runtime is not configured")

// Run dispatches a single invocation. Resource defaults are applied to
// the manifest in-place (on the copy used by the runner) so every
// runner sees a non-zero timeout / memory cap.
func (d *Dispatcher) Run(ctx context.Context, req Request) (Result, error) {
	if req.Skill.Runtime.TimeoutSeconds <= 0 {
		req.Skill.Runtime.TimeoutSeconds = DefaultTimeoutSeconds
	}
	if req.Skill.Runtime.MemoryMB <= 0 {
		req.Skill.Runtime.MemoryMB = DefaultMemoryMB
	}

	switch req.Skill.Runtime.Type {
	case models.RuntimeDocker:
		if d.docker == nil {
			err := fmt.Errorf("%w: docker", ErrRuntimeUnavailable)
			return Result{Status: StatusError, ErrorMessage: err.Error()}, err
		}
		return d.docker.Run(ctx, req)
	case models.RuntimeHTTPProxy:
		if d.http == nil {
			err := fmt.Errorf("%w: http_proxy", ErrRuntimeUnavailable)
			return Result{Status: StatusError, ErrorMessage: err.Error()}, err
		}
		return d.http.Run(ctx, req)
	default:
		err := fmt.Errorf("runtime: unknown runtime type %q", req.Skill.Runtime.Type)
		return Result{Status: StatusError, ErrorMessage: err.Error()}, err
	}
}
