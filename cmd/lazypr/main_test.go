package main

import "testing"

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    options
		wantErr bool
	}{
		{name: "no args", args: nil, want: options{}},
		{name: "pr number", args: []string{"123"}, want: options{PRNumber: 123}},
		{name: "repo flag", args: []string{"--repo", "owner/name"}, want: options{RepoOverride: "owner/name"}},
		{name: "url", args: []string{"https://github.com/owner/name/pull/77"}, want: options{RepoOverride: "owner/name", PRNumber: 77}},
		{name: "owner repo shorthand", args: []string{"owner/name#88"}, want: options{RepoOverride: "owner/name", PRNumber: 88}},
		{name: "too many args", args: []string{"1", "2"}, wantErr: true},
		{name: "bad arg", args: []string{"nope"}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseArgs error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %+v want %+v", got, tc.want)
			}
		})
	}
}

func TestSplitRepo(t *testing.T) {
	owner, repo, ok := splitRepo("a/b")
	if !ok || owner != "a" || repo != "b" {
		t.Fatalf("splitRepo failed: %q %q %v", owner, repo, ok)
	}
	if _, _, ok := splitRepo("a"); ok {
		t.Fatalf("expected invalid repo to fail")
	}
}
