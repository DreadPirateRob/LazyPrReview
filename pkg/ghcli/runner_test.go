package ghcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

type fakeCommand struct {
	output []byte
	err    error
}

func (c fakeCommand) CombinedOutput() ([]byte, error) {
	return c.output, c.err
}

func TestRunnerRunSuccess(t *testing.T) {
	prev := execCommand
	defer func() { execCommand = prev }()

	var gotLog CommandLogEntry
	execCommand = func(ctx context.Context, name string, args ...string) command {
		if name != "gh" {
			t.Fatalf("got command name %q", name)
		}
		if got := strings.Join(args, " "); got != "pr list --limit 1" {
			t.Fatalf("got args %q", got)
		}
		return fakeCommand{output: []byte(`{"ok":true}`)}
	}

	r := New(30*time.Second, func(entry CommandLogEntry) { gotLog = entry })
	out, err := r.Run(context.Background(), "pr", "list", "--limit", "1")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if string(out) != `{"ok":true}` {
		t.Fatalf("unexpected output %q", string(out))
	}
	if gotLog.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", gotLog.ExitCode)
	}
}

func TestRunnerRunGHMissing(t *testing.T) {
	prev := execCommand
	defer func() { execCommand = prev }()

	execCommand = func(ctx context.Context, name string, args ...string) command {
		return fakeCommand{err: exec.ErrNotFound}
	}

	r := New(30*time.Second, nil)
	_, err := r.Run(context.Background(), "auth", "status")
	if !errors.Is(err, ErrGHNotFound) {
		t.Fatalf("expected ErrGHNotFound, got %v", err)
	}
}

func TestRunnerRunTimeout(t *testing.T) {
	prev := execCommand
	defer func() { execCommand = prev }()

	execCommand = func(ctx context.Context, name string, args ...string) command {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestHelperProcessTimeout")
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
		return cmd
	}

	r := New(20*time.Millisecond, nil)
	_, err := r.Run(context.Background(), "auth", "status")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestRunnerRunNonZeroExit(t *testing.T) {
	prev := execCommand
	defer func() { execCommand = prev }()

	execCommand = func(ctx context.Context, name string, args ...string) command {
		return fakeCommand{err: &exec.ExitError{}, output: []byte("boom")}
	}

	r := New(30*time.Second, nil)
	_, err := r.Run(context.Background(), "repo", "view")
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T %v", err, err)
	}
}

func TestRunnerRunJSONGraphQLErrors(t *testing.T) {
	prev := execCommand
	defer func() { execCommand = prev }()

	execCommand = func(ctx context.Context, name string, args ...string) command {
		return fakeCommand{output: []byte(`{"errors":[{"message":"bad thing"}]}`)}
	}

	r := New(30*time.Second, nil)
	var out map[string]any
	err := r.RunJSON(context.Background(), &out, "api", "graphql", "-f", "query=query { viewer { login } }")
	var gqlErr *GraphQLError
	if !errors.As(err, &gqlErr) {
		t.Fatalf("expected GraphQLError, got %T %v", err, err)
	}
	if len(gqlErr.Messages) != 1 || gqlErr.Messages[0] != "bad thing" {
		t.Fatalf("unexpected messages: %#v", gqlErr.Messages)
	}
}

func TestHelperProcessTimeout(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	time.Sleep(200 * time.Millisecond)
	fmt.Fprintln(os.Stdout, "late")
	os.Exit(0)
}

func TestGhEnvScrubsColorHazards(t *testing.T) {
	t.Setenv("CLICOLOR_FORCE", "1")
	env := ghEnv()
	has := func(want string) bool {
		for _, e := range env {
			if e == want {
				return true
			}
		}
		return false
	}
	if has("CLICOLOR_FORCE=1") {
		t.Error("CLICOLOR_FORCE=1 must be scrubbed from child gh env")
	}
	for _, want := range []string{"NO_COLOR=1", "CLICOLOR_FORCE=0", "GH_PAGER=cat", "GH_PROMPT_DISABLED=1"} {
		if !has(want) {
			t.Errorf("child gh env missing %q", want)
		}
	}
}

func TestClassifyRateLimited(t *testing.T) {
	err := classifyAPIError([]string{"api"}, 1, "API rate limit exceeded for user ABC")
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
}
