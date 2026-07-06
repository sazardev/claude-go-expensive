package main

import (
	"path/filepath"
	"testing"
)

func TestSplitCommand(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCmd  string
		wantRest []string
	}{
		{"no args defaults to summary", nil, "summary", nil},
		{"bare command", []string{"sessions"}, "sessions", []string{}},
		// The regression this guards: flags typed *after* the subcommand
		// (the idiomatic order for git/docker/kubectl-style CLIs) must stay
		// attached to that subcommand's flag set, not get silently dropped.
		{"command with trailing flag", []string{"sessions", "-limit", "3"}, "sessions", []string{"-limit", "3"}},
		{"flag with no command defaults to summary", []string{"-limit", "3"}, "summary", []string{"-limit", "3"}},
		{"help flag with no command", []string{"--help"}, "summary", []string{"--help"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, rest := splitCommand(tt.args)
			if cmd != tt.wantCmd {
				t.Errorf("splitCommand(%v) cmd = %q; want %q", tt.args, cmd, tt.wantCmd)
			}
			if len(rest) != len(tt.wantRest) {
				t.Fatalf("splitCommand(%v) rest = %v; want %v", tt.args, rest, tt.wantRest)
			}
			for i := range rest {
				if rest[i] != tt.wantRest[i] {
					t.Errorf("splitCommand(%v) rest = %v; want %v", tt.args, rest, tt.wantRest)
				}
			}
		})
	}
}

func TestResolveDBPath(t *testing.T) {
	t.Run("explicit path wins", func(t *testing.T) {
		got, err := resolveDBPath("/tmp/custom.db")
		if err != nil {
			t.Fatalf("resolveDBPath() error = %v", err)
		}
		if got != "/tmp/custom.db" {
			t.Errorf("resolveDBPath(explicit) = %q; want %q", got, "/tmp/custom.db")
		}
	})

	t.Run("default is home-relative and cwd-independent", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		got, err := resolveDBPath("")
		if err != nil {
			t.Fatalf("resolveDBPath() error = %v", err)
		}
		want := filepath.Join(home, ".claude-cost", "expensive.db")
		if got != want {
			t.Errorf("resolveDBPath(\"\") = %q; want %q", got, want)
		}
	})
}
