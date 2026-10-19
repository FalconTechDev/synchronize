package syn

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
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
	// A background tool must never wait for an editor: `rebase --continue` and merges
	// open one for the commit message, and without a terminal that fails or hangs.
	"GIT_EDITOR=true",
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

// NetworkTimeout caps git commands that talk to a remote. Git has no timeout
// of its own: on a network that accepts the connection and then says nothing -
// a VPN dropping, a captive portal - it waits for ever, and a watcher running
// in the background waits with it.
var NetworkTimeout = networkTimeout()

func networkTimeout() time.Duration {
	if v := os.Getenv("SYN_NETWORK_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return 2 * time.Minute
}

func touchesNetwork(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "fetch", "push", "pull", "clone", "ls-remote":
		return true
	}
	return false
}

func gitEnvRun(dir string, extraEnv []string, args ...string) (string, error) {
	ctx := context.Background()
	if touchesNetwork(args) {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, NetworkTimeout)
		defer cancel()
	}

	// core.quotepath=false makes git print paths as they are. Left on - and
	// it is on by default - anything outside ASCII comes back as C-style
	// octal escapes and anything with a space comes back in quotes, so a
	// Russian filename reaches the log as Ð¸..., a hook's --paths
	// never matches it, and `syn ignore` cannot hold it back either.
	args = append([]string{"-c", "core.quotepath=false"}, args...)

	cmd := exec.CommandContext(ctx, "git", args...)
	// Killing git is not enough: it spawns git-remote-https, which keeps the
	// output pipes open, and Run would go on waiting for those. WaitDelay
	// closes them shortly after the context is done.
	cmd.WaitDelay = 5 * time.Second
	cmd.Dir = dir
	env := append(os.Environ(), gitBaseEnv...)
	env = append(env, extraEnv...)
	cmd.Env = env
	hideConsole(cmd)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("git %s gave up after %s - no answer from the remote\n"+
				"        check the network, or a VPN that has dropped",
				args[0], NetworkTimeout)
		}
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
	line = strings.TrimRight(line, " \t\r")
	i := strings.IndexByte(line, ' ')
	if i <= 0 {
		return "", "", false
	}
	code = line[:i]
	path = unquotePath(strings.TrimSpace(line[i:]))
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

// Diff shows what has not been committed yet - the thing you would lose, and
// the thing an automatic commit is about to take. Untracked files are included
// because syn commits with `add -A` and would sweep them in too.
func Diff(dir string) (string, error) {
	out, err := git(dir, "diff", "HEAD")
	if err != nil {
		return "", err
	}
	untracked, err := git(dir, "ls-files", "--others", "--exclude-standard")
	if err != nil || untracked == "" {
		return out, nil
	}
	var b strings.Builder
	b.WriteString(out)
	for _, f := range strings.Split(untracked, "\n") {
		if f == "" {
			continue
		}
		d, err := git(dir, "diff", "--no-index", "--", os.DevNull, f)
		if err != nil && d == "" {
			continue // --no-index exits non-zero whenever there is a difference
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(d)
	}
	return b.String(), nil
}

// DiffAgainstRemote shows what this branch carries that the remote does not.
func DiffAgainstRemote(dir, remote, branch string) (string, error) {
	if remote == "" || branch == "" {
		return git(dir, "diff", "HEAD")
	}
	ref := remote + "/" + branch
	if !gitOK(dir, "rev-parse", "--verify", "--quiet", ref) {
		return git(dir, "diff", "HEAD")
	}
	return git(dir, "diff", ref+"...HEAD")
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
		fs = append(fs, FileChange{label, unquotePath(strings.TrimSpace(parts[1]))})
	}
	return fs
}

func CurrentCommit(dir string) (string, error) {
	return git(dir, "rev-parse", "HEAD")
}

// RevParse turns any ref into the commit it names.
func RevParse(dir, ref string) (string, error) {
	out, err := git(dir, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// unquotePath turns git's quoted form of a path back into the path.
//
// git wraps a path in quotes and escapes it whenever it holds a space, a
// quote, a backslash or a control character - and, unless core.quotepath is
// off, anything outside ASCII as well. Left as-is, "my file.txt" is not the
// name of any file, so a hook's --paths never matches it, `syn ignore` cannot
// hold it back, and the log shows something nobody can copy.
func unquotePath(s string) string {
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return s
	}
	body := s[1 : len(s)-1]

	// Bytes, not runes: an escaped UTF-8 character arrives as several octal
	// escapes, one per byte, and only reassembles correctly byte by byte.
	out := make([]byte, 0, len(body))
	for i := 0; i < len(body); i++ {
		if body[i] != '\\' || i+1 >= len(body) {
			out = append(out, body[i])
			continue
		}
		i++
		switch c := body[i]; c {
		case 'n':
			out = append(out, '\n')
		case 't':
			out = append(out, '\t')
		case 'r':
			out = append(out, '\r')
		case 'a':
			out = append(out, 0x07)
		case 'b':
			out = append(out, 0x08)
		case 'f':
			out = append(out, 0x0c)
		case 'v':
			out = append(out, 0x0b)
		case '"', '\\':
			out = append(out, c)
		default:
			if c >= '0' && c <= '7' && i+2 < len(body) {
				n := 0
				digits := 0
				for ; digits < 3 && i < len(body) && body[i] >= '0' && body[i] <= '7'; digits++ {
					n = n*8 + int(body[i]-'0')
					i++
				}
				i-- // the loop above overshoots by one
				out = append(out, byte(n))
				continue
			}
			out = append(out, c)
		}
	}
	return string(out)
}
