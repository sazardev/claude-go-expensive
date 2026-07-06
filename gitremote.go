package expensive

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// detectProjectName derives a stable project name for repoRoot: the git
// remote "origin" URL when one is configured, so multiple clones or
// worktrees of the same repository roll up under one project instead of
// fragmenting by directory name (e.g. "widgets", "widgets-worktree-2", ...
// all sharing origin github.com/acme/widgets). Falls back to the directory
// name when there's no git remote to key on.
func detectProjectName(repoRoot string) string {
	if repoRoot == "" {
		return ""
	}
	if name, ok := gitRemoteProjectName(repoRoot); ok {
		return name
	}
	return filepath.Base(repoRoot)
}

func gitRemoteProjectName(repoRoot string) (string, bool) {
	configPath, ok := gitConfigPath(repoRoot)
	if !ok {
		return "", false
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", false
	}
	url, ok := parseOriginURL(string(data))
	if !ok {
		return "", false
	}
	return normalizeRemoteURL(url)
}

// gitConfigPath resolves the config file that holds repoRoot's remotes.
// Handles both a plain repo (.git is a directory) and a worktree checkout
// (.git is a file pointing at ".../.git/worktrees/<name>", whose remotes
// live two levels up in the main repo's config).
func gitConfigPath(repoRoot string) (string, bool) {
	gitPath := filepath.Join(repoRoot, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return "", false
	}
	if info.IsDir() {
		return filepath.Join(gitPath, "config"), true
	}

	data, err := os.ReadFile(gitPath)
	if err != nil {
		return "", false
	}
	const prefix = "gitdir:"
	line := strings.TrimSpace(string(data))
	if !strings.HasPrefix(line, prefix) {
		return "", false
	}
	worktreeGitDir := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if !filepath.IsAbs(worktreeGitDir) {
		worktreeGitDir = filepath.Join(repoRoot, worktreeGitDir)
	}
	mainGitDir := filepath.Dir(filepath.Dir(worktreeGitDir)) // .../worktrees/<name> -> .../.git
	return filepath.Join(mainGitDir, "config"), true
}

var (
	originSectionPattern = regexp.MustCompile(`(?s)\[remote "origin"\](.*?)(?:\n\[|\z)`)
	urlLinePattern       = regexp.MustCompile(`(?m)^\s*url\s*=\s*(.+?)\s*$`)
)

func parseOriginURL(config string) (string, bool) {
	section := originSectionPattern.FindStringSubmatch(config)
	if section == nil {
		return "", false
	}
	url := urlLinePattern.FindStringSubmatch(section[1])
	if url == nil {
		return "", false
	}
	return url[1], true
}

// normalizeRemoteURL turns a git remote URL into a short "owner/repo" style
// project name, e.g. "git@github.com:acme/widgets.git" -> "acme/widgets".
func normalizeRemoteURL(rawURL string) (string, bool) {
	s := strings.TrimSuffix(strings.TrimSpace(rawURL), ".git")

	switch {
	case strings.Contains(s, "://"):
		rest := strings.SplitN(s, "://", 2)[1]
		if at := strings.Index(rest, "@"); at != -1 {
			rest = rest[at+1:]
		}
		slash := strings.Index(rest, "/")
		if slash == -1 {
			return "", false
		}
		s = rest[slash+1:]
	case strings.Contains(s, "@") && strings.Contains(s, ":"):
		// scp-like syntax: git@host:owner/repo
		s = s[strings.Index(s, ":")+1:]
	default:
		return "", false
	}

	s = strings.Trim(s, "/")
	if s == "" {
		return "", false
	}
	return s, true
}
