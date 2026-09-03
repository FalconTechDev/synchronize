package syn

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
)

// GitError carries git's own stderr, which is what actually explains a failure.
type GitError struct {
	Args   []string
	Stderr string
	Err    error
}

func (e *GitError) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		msg = e.Err.Error()
	}
	return fmt.Sprintf("git %s: %s", strings.Join(e.Args, " "), msg)
}

func (e *GitError) Unwrap() error { return e.Err }

// gitEnv is the base environment every git invocation gets.
var gitBaseEnv = []string{
	"GIT_TERMINAL_PROMPT=0", // never block a boot/shutdown hook on a credential prompt
	"GIT_OPTIONAL_LOCKS=0",
}

// git runs a git command in dir and returns trimmed stdout.
func git(dir string, args ...string) (string, error) {
	return gitEnvRun(dir, nil, args...)
}

// gitWithToken runs git with an HTTPS credential helper that serves token for
// github.com, leaving the remote URL itself clean.
func gitWithToken(dir, token string, args ...string) (string, error) {
	if token == "" {
		return git(dir, args...)
	}
	// Inject via GIT_CONFIG env vars (Git 2.31+, widely available).
	// We first clear any system credential helpers (e.g. Git Credential Manager on
	// Windows) so they can't intercept the request, then inject our token helper.
	helper := fmt.Sprintf(`!f(){ echo username=oauth2; echo password=%s; }; f`, token)
	extra := []string{
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=credential.helper",
		"GIT_CONFIG_VALUE_0=",
		"GIT_CONFIG_KEY_1=credential.https://github.com.helper",
		"GIT_CONFIG_VALUE_1=" + helper,
	}
	return gitEnvRun(dir, extra, args...)
}

func gitEnvRun(dir string, extraEnv []string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	env := append(os.Environ(), gitBaseEnv...)
	env = append(env, extraEnv...)
	cmd.Env = env
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return strings.TrimSpace(out.String()), &GitError{Args: args, Stderr: errb.String(), Err: err}
	}
	return strings.TrimSpace(out.String()), nil
}

func gitOK(dir string, args ...string) bool {
	_, err := git(dir, args...)
	return err == nil
}

// IsRepo reports whether dir is inside a git working tree.
func IsRepo(dir string) bool {
	out, err := git(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && out == "true"
}

// RepoRoot returns the top-level directory of the working tree containing dir.
func RepoRoot(dir string) (string, error) {
	return git(dir, "rev-parse", "--show-toplevel")
}

// CurrentBranch returns the checked-out branch, or an error in detached HEAD.
func CurrentBranch(dir string) (string, error) {
	b, err := git(dir, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return "", fmt.Errorf("detached HEAD (not on a branch)")
	}
	return b, nil
}

// IsDirty reports whether there is anything to commit, untracked files included.
func IsDirty(dir string) (bool, error) {
	out, err := git(dir, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return false, err
	}
	return out != "", nil
}

// ChangedFiles lists the paths that differ from HEAD.
func ChangedFiles(dir string) ([]string, error) {
	out, err := git(dir, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(out, "\n") {
		if len(line) > 3 {
			files = append(files, strings.TrimSpace(line[3:]))
		}
	}
	return files, nil
}

// Remotes lists configured remote names.
func Remotes(dir string) ([]string, error) {
	out, err := git(dir, "remote")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// RemoteURL returns the fetch URL of a remote.
func RemoteURL(dir, remote string) (string, error) {
	return git(dir, "remote", "get-url", remote)
}

// AheadBehind counts commits the local branch has that the remote does not,
// and vice versa.
func AheadBehind(dir, branch, remote string) (ahead, behind int, err error) {
	ref := remote + "/" + branch
	if !gitOK(dir, "rev-parse", "--verify", "--quiet", ref) {
		// No remote branch yet: everything local is "ahead".
		n, cerr := git(dir, "rev-list", "--count", "HEAD")
		if cerr != nil {
			return 0, 0, cerr
		}
		fmt.Sscanf(n, "%d", &ahead)
		return ahead, 0, nil
	}
	out, err := git(dir, "rev-list", "--left-right", "--count", "HEAD..."+ref)
	if err != nil {
		return 0, 0, err
	}
	fmt.Sscanf(out, "%d\t%d", &ahead, &behind)
	return ahead, behind, nil
}

// RemoteBranchExists reports whether remote/branch is known locally after a fetch.
func RemoteBranchExists(dir, remote, branch string) bool {
	return gitOK(dir, "rev-parse", "--verify", "--quiet", remote+"/"+branch)
}

// InProgress reports whether a rebase or merge was left half-finished.
func InProgress(dir string) string {
	gitDir, err := git(dir, "rev-parse", "--git-dir")
	if err != nil {
		return ""
	}
	if !strings.Contains(gitDir, ":") && !strings.HasPrefix(gitDir, "/") {
		gitDir = dir + "/" + gitDir
	}
	for name, label := range map[string]string{
		"rebase-merge":     "rebase",
		"rebase-apply":     "rebase",
		"MERGE_HEAD":       "merge",
		"CHERRY_PICK_HEAD": "cherry-pick",
	} {
		if _, err := os.Stat(gitDir + "/" + name); err == nil {
			return label
		}
	}
	return ""
}

// ConflictedFiles lists paths with unresolved merge conflicts.
func ConflictedFiles(dir string) []string {
	out, err := git(dir, "diff", "--name-only", "--diff-filter=U")
	if err != nil || out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

// GitVersion returns the installed git version string.
func GitVersion() (string, error) {
	return git(".", "--version")
}

// Diff returns the diff of a repo against remote or HEAD.
// If remote is set, shows what would be pushed/pulled; otherwise shows
// uncommitted changes.
func Diff(dir, remote, branch string) (string, error) {
	if remote != "" && branch != "" {
		ref := remote + "/" + branch
		if gitOK(dir, "rev-parse", "--verify", "--quiet", ref) {
			return git(dir, "diff", ref+"...HEAD")
		}
	}
	return git(dir, "diff", "HEAD")
}

// RemoteHost parses the hostname from a git remote URL.
// Handles https://github.com/..., git@github.com:..., and ssh://... forms.
func RemoteHost(rawURL string) string {
	// SSH shorthand: git@github.com:owner/repo
	if strings.HasPrefix(rawURL, "git@") {
		rawURL = strings.TrimPrefix(rawURL, "git@")
		if i := strings.IndexByte(rawURL, ':'); i >= 0 {
			return rawURL[:i]
		}
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// CloneWithToken clones url into dest, injecting token as an HTTPS credential
// for github.com so the URL stored in the new repo's .git/config stays clean.
func CloneWithToken(cloneURL, dest, token string, extraArgs ...string) error {
	args := append([]string{"clone"}, extraArgs...)
	args = append(args, cloneURL, dest)
	_, err := gitWithToken(".", token, args...)
	return err
}
