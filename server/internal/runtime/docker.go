package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Docker runs a skill in a one-shot container. The container is
// sandboxed:
//
//   - --rm: removed automatically after the call
//   - --network=none: no outbound network (M2 default — egress allow-list
//     comes later when we add manifest.network)
//   - --read-only: read-only root filesystem
//   - --cap-drop ALL --security-opt no-new-privileges: minimal kernel
//     capabilities
//   - --user nobody: never run as root
//   - --memory / --cpus / --pids-limit: hard resource caps
//
// Input JSON is piped to the container's stdin; the container is
// expected to write a JSON object to stdout. The dispatcher applies a
// hard timeout: when the context expires, the docker CLI is killed and
// the runner returns a `timeout` result.
//
// The platform shells out to the `docker` binary rather than embedding
// the Docker SDK. The SDK pulls in a lot of dependencies (containerd,
// buildkit clients, ...) that we don't need for one-shot exec. CLI
// shell-out is straightforward, well-documented, and identical to what
// an operator would type at a prompt — making it easier to debug
// behaviour in production.
type Docker struct {
	binary string
	// runCmd is a hook for unit tests. Production code leaves it nil
	// and the runner invokes os/exec.CommandContext directly. Tests
	// inject a fake to assert which arguments were assembled without
	// having to spin up a real docker daemon.
	runCmd func(ctx context.Context, name string, args []string, stdin []byte) (stdout, stderr []byte, exitCode int, err error)
}

// NewDocker returns a Runner that shells out to the docker CLI. Pass
// the path to the docker binary (typically "docker"). Returns an error
// when the binary cannot be located.
func NewDocker(binary string) (*Docker, error) {
	if binary == "" {
		binary = "docker"
	}
	if _, err := exec.LookPath(binary); err != nil {
		return nil, fmt.Errorf("docker runtime: %w", err)
	}
	return &Docker{binary: binary}, nil
}

// Run executes the docker container.
func (d *Docker) Run(ctx context.Context, req Request) (Result, error) {
	image := req.Skill.Runtime.Image
	if image == "" {
		return Result{Status: StatusError, ErrorMessage: "runtime.image is empty"},
			errors.New("docker: runtime.image is empty")
	}
	timeout := time.Duration(req.Skill.Runtime.TimeoutSeconds) * time.Second
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	input := req.Input
	if input == nil {
		input = map[string]any{}
	}
	stdin, err := json.Marshal(input)
	if err != nil {
		return Result{Status: StatusError, ErrorMessage: err.Error()}, err
	}

	args := []string{
		"run", "--rm", "-i",
		"--network=none",
		"--read-only",
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--user", "nobody",
		"--pids-limit", "128",
		"--memory", strconv.Itoa(req.Skill.Runtime.MemoryMB) + "m",
		"--cpus", "1.0",
	}
	// Docker's --entrypoint flag only accepts the executable name; any
	// additional words (e.g. `python -m hello`) must be appended as CMD
	// arguments *after* the image name. Splitting on whitespace mirrors
	// how a user would type the command at a prompt and matches the
	// `ENTRYPOINT` shell-form convention used in the example skills.
	var entrypointArgs []string
	if ep := strings.TrimSpace(req.Skill.Runtime.Entrypoint); ep != "" {
		parts := strings.Fields(ep)
		args = append(args, "--entrypoint", parts[0])
		entrypointArgs = parts[1:]
	}
	args = append(args, image)
	args = append(args, entrypointArgs...)

	stdout, stderr, exitCode, runErr := d.invokeDocker(runCtx, d.binary, args, stdin)

	// Context-cancelled-due-to-timeout is the dispatch-level deadline.
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return Result{
			Status:       StatusTimeout,
			ErrorMessage: fmt.Sprintf("docker container exceeded timeout of %s", timeout),
			OutputBytes:  len(stdout),
		}, nil
	}
	if runErr != nil {
		return Result{
			Status:       StatusError,
			ErrorMessage: fmt.Sprintf("docker run failed (exit %d): %s: %v", exitCode, truncate(string(stderr), 200), runErr),
			OutputBytes:  len(stdout),
		}, nil
	}
	if exitCode != 0 {
		return Result{
			Status:       StatusError,
			ErrorMessage: fmt.Sprintf("skill exited %d: %s", exitCode, truncate(string(stderr), 200)),
			OutputBytes:  len(stdout),
		}, nil
	}
	if len(stdout) > MaxOutputBytes {
		return Result{
			Status:       StatusError,
			ErrorMessage: fmt.Sprintf("skill stdout exceeded %d bytes", MaxOutputBytes),
			OutputBytes:  len(stdout),
		}, nil
	}

	out := map[string]any{}
	if len(bytes.TrimSpace(stdout)) > 0 {
		if err := json.Unmarshal(stdout, &out); err != nil {
			return Result{
				Status:       StatusError,
				ErrorMessage: fmt.Sprintf("skill returned non-JSON stdout: %v", err),
				OutputBytes:  len(stdout),
			}, nil
		}
	}
	return Result{
		Status:      StatusOK,
		Output:      out,
		OutputBytes: len(stdout),
	}, nil
}

// invokeDocker is the seam unit tests substitute. Production runs the
// real binary via os/exec.
func (d *Docker) invokeDocker(ctx context.Context, binary string, args []string, stdin []byte) ([]byte, []byte, int, error) {
	if d.runCmd != nil {
		return d.runCmd(ctx, binary, args, stdin)
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	var stdout, stderr bytes.Buffer
	// Cap stdout at MaxOutputBytes+1 so we can detect (rather than just
	// truncate) over-large skill output and return an error.
	cmd.Stdout = &limitedWriter{w: &stdout, max: MaxOutputBytes + 1}
	cmd.Stderr = &stderr
	cmd.Stdin = bytes.NewReader(stdin)
	err := cmd.Run()
	exit := 0
	if cmd.ProcessState != nil {
		exit = cmd.ProcessState.ExitCode()
	}
	return stdout.Bytes(), stderr.Bytes(), exit, err
}

// limitedWriter is an io.Writer that stops accepting bytes after max.
// Used to prevent runaway skill output from filling memory. The pointer
// receiver on Write is deliberate — we track cumulative bytes across
// successive writes within one process.
type limitedWriter struct {
	w   io.Writer
	max int
	n   int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	remaining := l.max - l.n
	if remaining <= 0 {
		// Pretend the write succeeded so the child process doesn't
		// abort with SIGPIPE; we'll reject the call after-the-fact
		// because the inspector sees stdout > MaxOutputBytes.
		return len(p), nil
	}
	if len(p) > remaining {
		_, err := l.w.Write(p[:remaining])
		if err != nil {
			return 0, err
		}
		l.n += len(p)
		return len(p), nil
	}
	n, err := l.w.Write(p)
	l.n += n
	return n, err
}
