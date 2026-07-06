package expensive

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeRemoteURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
		ok   bool
	}{
		{"https", "https://github.com/acme/widgets.git", "acme/widgets", true},
		{"https no dot git", "https://github.com/acme/widgets", "acme/widgets", true},
		{"scp-like ssh", "git@github.com:acme/widgets.git", "acme/widgets", true},
		{"explicit ssh scheme", "ssh://git@github.com/acme/widgets.git", "acme/widgets", true},
		{"gitlab nested groups", "https://gitlab.com/team/sub/widgets.git", "team/sub/widgets", true},
		{"no host separator", "justaword", "", false},
		{"empty", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := normalizeRemoteURL(tt.url)
			if ok != tt.ok || got != tt.want {
				t.Errorf("normalizeRemoteURL(%q) = (%q, %v); want (%q, %v)", tt.url, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func writeGitConfig(t *testing.T, repoRoot, remoteURL string) {
	t.Helper()
	gitDir := filepath.Join(repoRoot, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	config := "[core]\n\trepositoryformatversion = 0\n[remote \"origin\"]\n\turl = " + remoteURL + "\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n[branch \"main\"]\n\tremote = origin\n"
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(config), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func TestDetectProjectNameFromGitRemote(t *testing.T) {
	root := t.TempDir()
	writeGitConfig(t, root, "https://github.com/acme/widgets.git")

	if got := detectProjectName(root); got != "acme/widgets" {
		t.Errorf("detectProjectName() = %q; want %q", got, "acme/widgets")
	}
}

func TestDetectProjectNameMultipleClonesShareOrigin(t *testing.T) {
	base := t.TempDir()
	main := filepath.Join(base, "widgets")
	clone := filepath.Join(base, "widgets-clone-2")
	writeGitConfig(t, main, "git@github.com:acme/widgets.git")
	writeGitConfig(t, clone, "git@github.com:acme/widgets.git")

	got1 := detectProjectName(main)
	got2 := detectProjectName(clone)
	if got1 != got2 {
		t.Errorf("detectProjectName() diverged across clones of the same origin: %q vs %q", got1, got2)
	}
	if got1 != "acme/widgets" {
		t.Errorf("detectProjectName() = %q; want %q", got1, "acme/widgets")
	}
}

func TestDetectProjectNameWorktree(t *testing.T) {
	base := t.TempDir()
	mainRepo := filepath.Join(base, "main")
	writeGitConfig(t, mainRepo, "https://github.com/acme/widgets.git")

	worktreeGitDir := filepath.Join(mainRepo, ".git", "worktrees", "feature")
	if err := os.MkdirAll(worktreeGitDir, 0o755); err != nil {
		t.Fatalf("mkdir worktree gitdir: %v", err)
	}
	worktree := filepath.Join(base, "feature-worktree")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatalf("mkdir worktree: %v", err)
	}
	pointer := "gitdir: " + worktreeGitDir + "\n"
	if err := os.WriteFile(filepath.Join(worktree, ".git"), []byte(pointer), 0o644); err != nil {
		t.Fatalf("write .git pointer: %v", err)
	}

	if got := detectProjectName(worktree); got != "acme/widgets" {
		t.Errorf("detectProjectName(worktree) = %q; want %q (resolved via main repo config)", got, "acme/widgets")
	}
}

func TestDetectProjectNameFallsBackWithoutGit(t *testing.T) {
	root := filepath.Join(t.TempDir(), "no-git-here")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if got := detectProjectName(root); got != "no-git-here" {
		t.Errorf("detectProjectName() = %q; want directory basename %q", got, "no-git-here")
	}
}
