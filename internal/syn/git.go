package syn

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
)

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

var gitBaseEnv = []string{
	"GIT_TERMINAL_PROMPT=0", 
	"GIT_OPTIONAL_LOCKS=0",
}

func git(dir string, args ...string) (string, error) {
	return gitEnvRun(dir, nil, args...)
}

var FallbackIdentity Identity

func gitCommit(dir, msg string) (string, error) {
	return gitEnvRun(dir, identityEnv(dir), "commit", "--quiet", "-m", msg)
}

func identityEnv(dir string) []string {
	if FallbackIdentity.IsZero() {
		return nil
	}
	if hasGitIdentity(dir) {
		return nil
	}
	Debug("git identity not configured - using your GitHub identity (%s <%s>)",
		FallbackIdentity.Name, FallbackIdentity.Email)
	return []string{
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=user.name",
		"GIT_CONFIG_VALUE_0=" + FallbackIdentity.Name,
		"GIT_CONFIG_KEY_1=user.email",
		"GIT_CONFIG_VALUE_1=" + FallbackIdentity.Email,
	}
}

func hasGitIdentity(dir string) bool {
	name, err := git(dir, "config", "--get", "user.name")
	if err != nil || strings.TrimSpace(name) == "" {
		return false
	}
	email, err := git(dir, "config", "--get", "user.email")
	return err == nil && strings.TrimSpace(email) != ""
}

func gitWithToken(dir, token string, args ...string) (string, error) {
	if token == "" {
		return git(dir, args...)
	}
	
	
	
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
	hideConsole(cmd)
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

func IsRepo(dir string) bool {
	out, err := git(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && out == "true"
}

func RepoRoot(dir string) (string, error) {
	return git(dir, "rev-parse", "--show-toplevel")
}

func CurrentBranch(dir string) (string, error) {
	b, err := git(dir, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return "", fmt.Errorf("detached HEAD (not on a branch)")
	}
	return b, nil
}

func IsDirty(dir string) (bool, error) {
	out, err := git(dir, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return false, err
	}
	return out != "", nil
}

func ChangedFiles(dir string) ([]string, error) {
	out, err := git(dir, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(out, "\n") {
		if _, path, ok := splitStatusLine(line); ok {
			files = append(files, path)
		}
	}
	return files, nil
}

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

func RemoteURL(dir, remote string) (string, error) {
	return git(dir, "remote", "get-url", remote)
}

func AheadBehind(dir, branch, remote string) (ahead, behind int, err error) {
	ref := remote + "/" + branch
	if !gitOK(dir, "rev-parse", "--verify", "--quiet", ref) {
		
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

func RemoteBranchExists(dir, remote, branch string) bool {
	return gitOK(dir, "rev-parse", "--verify", "--quiet", remote+"/"+branch)
}

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

func ConflictedFiles(dir string) []string {
	out, err := git(dir, "diff", "--name-only", "--diff-filter=U")
	if err != nil || out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

func GitVersion() (string, error) {
	return git(".", "--version")
}

type Commit struct {
	Hash    string `json:"hash"`
	Subject string `json:"subject"`
	Author  string `json:"author"`
	When    string `json:"when"`
}

func RecentCommits(dir string, limit int) []Commit {
	out, err := git(dir, "log", fmt.Sprintf("--max-count=%d", limit),
		"--pretty=format:%h\x1f%s\x1f%an\x1f%cr")
	if err != nil {
		return nil
	}
	var cs []Commit
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Split(line, "\x1f")
		if len(f) != 4 {
			continue
		}
		cs = append(cs, Commit{f[0], f[1], f[2], f[3]})
	}
	return cs
}

// splitStatusLine reads one porcelain line. It cannot use fixed offsets: the
// leading space of an unstaged change is trimmed away before we see it, which
// used to shift every path by one character.
func splitStatusLine(line string) (code, path string, ok bool) {
	line = strings.TrimRight(line, " 	")
	i := strings.IndexByte(line, ' ')
	if i <= 0 {
		return "", "", false
	}
	code = line[:i]
	path = strings.TrimSpace(line[i:])
	return code, path, path != ""
}

type FileChange struct {
	Status string `json:"status"`
	Path   string `json:"path"`
}

func ChangedFilesDetailed(dir string) []FileChange {
	out, err := git(dir, "status", "--porcelain=v1")
	if err != nil {
		return nil
	}
	var fs []FileChange
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		code, path, ok := splitStatusLine(line)
		if !ok {
			continue
		}
		label := "modified"
		switch {
		case strings.HasPrefix(code, "??"):
			label = "new"
		case strings.HasPrefix(code, "A"):
			label = "added"
		case strings.HasPrefix(code, "D"):
			label = "deleted"
		case strings.HasPrefix(code, "R"):
			label = "renamed"
		}
		fs = append(fs, FileChange{label, path})
	}
	return fs
}

func CommitLog(dir, from, to string, limit int) []string {
	out, err := git(dir, "log", "--no-merges", fmt.Sprintf("--max-count=%d", limit),
		"--pretty=format:%h  %s", from+".."+to)
	if err != nil {
		return nil
	}
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

func Diff(dir, remote, branch string) (string, error) {
	if remote != "" && branch != "" {
		ref := remote + "/" + branch
		if gitOK(dir, "rev-parse", "--verify", "--quiet", ref) {
			return git(dir, "diff", ref+"...HEAD")
		}
	}
	return git(dir, "diff", "HEAD")
}

func RemoteHost(rawURL string) string {
	
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

func CloneWithToken(cloneURL, dest, token string, extraArgs ...string) error {
	args := append([]string{"clone"}, extraArgs...)
	args = append(args, cloneURL, dest)
	_, err := gitWithToken(".", token, args...)
	return err
}

// FilesBetween lists what changed between two commits, so a sync can say
// which files arrived rather than only how many commits did.
func FilesBetween(dir, from, to string) []FileChange {
	out, err := git(dir, "diff", "--name-status", from, to)
	if err != nil || out == "" {
		return nil
	}
	var fs []FileChange
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		label := "modified"
		switch parts[0][:1] {
		case "A":
			label = "added"
		case "D":
			label = "deleted"
		case "R":
			label = "renamed"
		}
		fs = append(fs, FileChange{label, strings.TrimSpace(parts[1])})
	}
	return fs
}

func CurrentCommit(dir string) (string, error) {
	return git(dir, "rev-parse", "HEAD")
}
