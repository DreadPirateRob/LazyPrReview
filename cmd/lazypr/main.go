package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/DreadPirateRob/LazyPrReview/pkg/config"
	"github.com/DreadPirateRob/LazyPrReview/pkg/forge"
	ghforge "github.com/DreadPirateRob/LazyPrReview/pkg/forge/github"
	"github.com/DreadPirateRob/LazyPrReview/pkg/ghcli"
	"github.com/DreadPirateRob/LazyPrReview/pkg/ui"
)

type options struct {
	RepoOverride string
	Debug        bool
	PRNumber     int
}

var prURLRE = regexp.MustCompile(`^https://github\.com/([^/]+)/([^/]+)/pull/(\d+)$`)
var repoPRRE = regexp.MustCompile(`^([^/]+)/([^#]+)#(\d+)$`)

func main() {
	opts, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = opts.Debug

	logCh := make(chan ghcli.CommandLogEntry, 64)
	runner := ghcli.New(30*time.Second, func(entry ghcli.CommandLogEntry) {
		select {
		case logCh <- entry:
		default:
		}
	})
	client := ghforge.New(runner)
	model := ui.New(cfg, client)
	model.OpenedPRNumber = opts.PRNumber
	model.StartupCmd = startupCmd(client, opts)

	p := tea.NewProgram(model)
	go func() {
		for entry := range logCh {
			p.Send(ui.CommandLogEntryMsg{Entry: entry})
		}
	}()

	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	close(logCh)
}

func parseArgs(args []string) (options, error) {
	fs := flag.NewFlagSet("lazypr", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var opts options
	fs.StringVar(&opts.RepoOverride, "repo", "", "owner/name")
	fs.BoolVar(&opts.Debug, "debug", false, "write debug log")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	rest := fs.Args()
	if len(rest) > 1 {
		return options{}, errors.New("expected at most one positional argument")
	}
	if len(rest) == 0 {
		return opts, nil
	}
	arg := rest[0]
	if n, err := strconv.Atoi(arg); err == nil && n > 0 {
		opts.PRNumber = n
		return opts, nil
	}
	if m := prURLRE.FindStringSubmatch(arg); m != nil {
		opts.RepoOverride = m[1] + "/" + m[2]
		n, _ := strconv.Atoi(m[3])
		opts.PRNumber = n
		return opts, nil
	}
	if m := repoPRRE.FindStringSubmatch(arg); m != nil {
		opts.RepoOverride = m[1] + "/" + m[2]
		n, _ := strconv.Atoi(m[3])
		opts.PRNumber = n
		return opts, nil
	}
	return options{}, fmt.Errorf("invalid PR argument %q", arg)
}

func startupCmd(client *ghforge.Client, opts options) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		if err := client.ProbeAuth(ctx); err != nil {
			switch {
			case errors.Is(err, ghcli.ErrGHNotFound):
				return ui.StartupLoadedMsg{Fatal: ui.FatalMissingGH, Detail: err.Error()}
			case errors.Is(err, ghcli.ErrGHUnauthed):
				return ui.StartupLoadedMsg{Fatal: ui.FatalUnauthed, Detail: err.Error()}
			default:
				return ui.StartupLoadedMsg{Fatal: ui.FatalUnauthed, Detail: err.Error()}
			}
		}
		viewer, err := client.ViewerLogin(ctx)
		if err != nil {
			return ui.StartupLoadedMsg{Fatal: ui.FatalUnauthed, Detail: err.Error()}
		}
		repo, err := resolveRepo(ctx, client, opts.RepoOverride)
		if err != nil {
			return ui.StartupLoadedMsg{Fatal: ui.FatalNoRepo, Detail: err.Error()}
		}
		prs, err := client.ListPRs(ctx, repo, filterForStartup())
		if err != nil {
			return ui.StartupLoadedMsg{Fatal: ui.FatalNoRepo, Detail: err.Error()}
		}
		return ui.StartupLoadedMsg{Repo: repo, PRs: prs, ViewerLogin: viewer}
	}
}

func resolveRepo(ctx context.Context, client *ghforge.Client, override string) (forge.Repo, error) {
	if override == "" {
		return client.ResolveRepo(ctx)
	}
	owner, name, ok := splitRepo(override)
	if !ok {
		return forge.Repo{}, fmt.Errorf("invalid --repo value %q", override)
	}
	return forge.Repo{Owner: owner, Name: name}, nil
}

func filterForStartup() forge.PRFilter {
	return forge.FilterReviewRequested
}

func splitRepo(s string) (string, string, bool) {
	parts := strings.Split(s, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
