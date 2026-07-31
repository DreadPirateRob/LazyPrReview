package ghcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

var (
	ErrGHNotFound          = errors.New("gh not found")
	ErrGHUnauthed          = errors.New("gh unauthenticated")
	ErrNoRepo              = errors.New("no repo resolvable")
	ErrUnsupportedInPhase1 = errors.New("unsupported in phase 1")
	ErrRateLimited         = errors.New("github api rate limit reached")
)

type APIError struct {
	Command  []string
	ExitCode int
	Stderr   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("gh command failed with exit code %d", e.ExitCode)
}

type GraphQLError struct {
	Messages []string
	Raw      string
}

func (e *GraphQLError) Error() string {
	if len(e.Messages) == 0 {
		return "graphql error"
	}
	return "graphql error: " + strings.Join(e.Messages, "; ")
}

type CommandLogEntry struct {
	Command  []string
	Duration time.Duration
	ExitCode int
	Stderr   string
	Err      string
}

type Runner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
	RunJSON(ctx context.Context, out any, args ...string) error
}

type runner struct {
	timeout time.Duration
	tap     func(CommandLogEntry)
}

type command interface {
	CombinedOutput() ([]byte, error)
}

var execCommand = func(ctx context.Context, name string, args ...string) command {
	c := exec.CommandContext(ctx, name, args...)
	c.Env = ghEnv()
	return c
}

// ghEnv returns the parent environment with color/pager/prompt hazards
// neutralized for child gh calls. CLICOLOR_FORCE=1 in particular makes gh
// colorize non-TTY output and corrupts parsing of `gh auth status` text
// (observed as a false "not authenticated").
func ghEnv() []string {
	base := os.Environ()
	out := make([]string, 0, len(base)+5)
	for _, e := range base {
		switch {
		case strings.HasPrefix(e, "CLICOLOR_FORCE="),
			strings.HasPrefix(e, "CLICOLOR="),
			strings.HasPrefix(e, "NO_COLOR="),
			strings.HasPrefix(e, "GH_PAGER="),
			strings.HasPrefix(e, "GH_PROMPT_DISABLED="):
			continue
		}
		out = append(out, e)
	}
	return append(out,
		"NO_COLOR=1",
		"CLICOLOR_FORCE=0",
		"GH_PAGER=cat",
		"GH_PROMPT_DISABLED=1",
		"GH_NO_UPDATE_NOTIFIER=1",
	)
}

func New(timeout time.Duration, tap func(CommandLogEntry)) Runner {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &runner{timeout: timeout, tap: tap}
}

func (r *runner) Run(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	start := time.Now()
	cmd := execCommand(ctx, "gh", args...)
	out, err := cmd.CombinedOutput()
	duration := time.Since(start)

	entry := CommandLogEntry{Command: append([]string{"gh"}, args...), Duration: duration}
	if err == nil {
		r.log(entry)
		return out, nil
	}

	entry.Err = err.Error()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		entry.Stderr = strings.TrimSpace(string(out))
		r.log(entry)
		return nil, context.DeadlineExceeded
	}
	if errors.Is(err, exec.ErrNotFound) {
		entry.Stderr = strings.TrimSpace(string(out))
		r.log(entry)
		return nil, ErrGHNotFound
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		entry.ExitCode = exitErr.ExitCode()
		entry.Stderr = strings.TrimSpace(string(exitErr.Stderr))
		if entry.Stderr == "" {
			entry.Stderr = strings.TrimSpace(string(out))
		}
		classified := classifyAPIError(args, entry.ExitCode, entry.Stderr)
		r.log(entry)
		return nil, classified
	}
	entry.Stderr = strings.TrimSpace(string(out))
	r.log(entry)
	return nil, err
}

func (r *runner) RunJSON(ctx context.Context, out any, args ...string) error {
	body, err := r.Run(ctx, args...)
	if err != nil {
		return err
	}
	if isGraphQLCall(args) {
		if gqlErr := parseGraphQLErrors(body); gqlErr != nil {
			return gqlErr
		}
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	return nil
}

func (r *runner) log(entry CommandLogEntry) {
	if r.tap != nil {
		r.tap(entry)
	}
}

func classifyAPIError(args []string, exitCode int, stderr string) error {
	msg := strings.ToLower(stderr)
	switch {
	case strings.Contains(msg, "authenticate") || strings.Contains(msg, "not logged into") || strings.Contains(msg, "gh auth login"):
		return fmt.Errorf("%w: %s", ErrGHUnauthed, stderr)
	case strings.Contains(msg, "not a git repository") || strings.Contains(msg, "run this command from within a git repository") || strings.Contains(msg, "could not determine base repository"):
		return fmt.Errorf("%w: %s", ErrNoRepo, stderr)
	case strings.Contains(msg, "api rate limit exceeded") || strings.Contains(msg, "secondary rate limit") || strings.Contains(msg, "was submitted too quickly") || strings.Contains(msg, "rate limit"):
		return fmt.Errorf("%w: %s", ErrRateLimited, stderr)
	default:
		return &APIError{Command: append([]string{"gh"}, args...), ExitCode: exitCode, Stderr: stderr}
	}
}

func isGraphQLCall(args []string) bool {
	if len(args) < 2 {
		return false
	}
	return args[0] == "api" && args[1] == "graphql"
}

func parseGraphQLErrors(body []byte) error {
	var envelope struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil
	}
	if len(envelope.Errors) == 0 {
		return nil
	}
	messages := make([]string, 0, len(envelope.Errors))
	for _, item := range envelope.Errors {
		if item.Message != "" {
			messages = append(messages, item.Message)
		}
	}
	return &GraphQLError{Messages: messages, Raw: string(body)}
}
